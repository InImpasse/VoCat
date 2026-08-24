package server

import (
	"container/list"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"sync"
	"time"
)

const (
	loginPairMaxFailures    = 5
	loginAccountMaxFailures = 10
	loginIPMaxFailures      = 20
	loginLimiterMaxEntries  = 4096
	loginBcryptConcurrency  = 2
	loginBcryptBusyRetry    = time.Second
	loginLockoutAuditPeriod = time.Minute
)

// loginRateLimiter blunts online brute-force and username-flood attacks before
// they reach bcrypt. Per-IP, per-account, and combined limits share a bounded
// LRU state table, while password checks have their own small concurrency cap.
type loginRateLimiter struct {
	mu          sync.Mutex
	attempts    map[string]*loginAttempt
	recency     *list.List
	maxEntries  int
	window      time.Duration
	lockout     time.Duration
	now         func() time.Time
	bcryptSlots chan struct{}
}

type loginAttempt struct {
	failures    int
	firstFail   time.Time
	lockedUntil time.Time
	lockAuditAt time.Time
	inFlight    int
	recency     *list.Element
}

type loginBucket struct {
	key         string
	maxFailures int
}

func newLoginRateLimiter() *loginRateLimiter {
	return &loginRateLimiter{
		attempts:    make(map[string]*loginAttempt),
		recency:     list.New(),
		maxEntries:  loginLimiterMaxEntries,
		window:      10 * time.Minute,
		lockout:     10 * time.Minute,
		now:         time.Now,
		bcryptSlots: make(chan struct{}, loginBcryptConcurrency),
	}
}

// begin admits one password verification. A caller that is admitted must call
// end, normally with defer, after the authentication attempt completes.
func (l *loginRateLimiter) begin(ip string, username string) (time.Duration, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	retryAfter := time.Duration(0)
	for _, bucket := range loginBuckets(ip, username) {
		attempt, ok := l.attempts[bucket.key]
		if !ok {
			continue
		}
		l.touch(attempt)
		if remaining := attempt.lockedUntil.Sub(now); remaining > retryAfter {
			retryAfter = remaining
		}
	}
	if retryAfter > 0 {
		return retryAfter, false
	}

	select {
	case l.bcryptSlots <- struct{}{}:
	default:
		return loginBcryptBusyRetry, false
	}

	buckets := loginBuckets(ip, username)
	if !l.ensureBuckets(buckets, now) {
		<-l.bcryptSlots
		return loginBcryptBusyRetry, false
	}
	for _, bucket := range buckets {
		l.attempts[bucket.key].inFlight++
	}
	return 0, true
}

func (l *loginRateLimiter) end(ip string, username string) {
	l.mu.Lock()
	now := l.now()
	for _, bucket := range loginBuckets(ip, username) {
		attempt, ok := l.attempts[bucket.key]
		if !ok {
			continue
		}
		if attempt.inFlight > 0 {
			attempt.inFlight--
		}
		if !l.active(attempt, now) {
			l.delete(bucket.key)
		}
	}
	l.mu.Unlock()
	<-l.bcryptSlots
}

// recordFailure registers a failed attempt against all three dimensions. It
// returns the longest active lock when any threshold has been reached.
func (l *loginRateLimiter) recordFailure(ip string, username string) (time.Duration, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	buckets := loginBuckets(ip, username)
	if !l.ensureBuckets(buckets, now) {
		return loginBcryptBusyRetry, true
	}
	retryAfter := time.Duration(0)
	for _, bucket := range buckets {
		attempt := l.attempts[bucket.key]
		l.touch(attempt)
		if remaining := attempt.lockedUntil.Sub(now); remaining > 0 {
			if remaining > retryAfter {
				retryAfter = remaining
			}
			continue
		}
		if attempt.firstFail.IsZero() || now.Sub(attempt.firstFail) > l.window {
			attempt.failures = 0
			attempt.firstFail = now
		}
		attempt.failures++
		if attempt.failures >= bucket.maxFailures {
			attempt.failures = 0
			attempt.firstFail = now
			attempt.lockedUntil = now.Add(l.lockout)
			attempt.lockAuditAt = time.Time{}
			if l.lockout > retryAfter {
				retryAfter = l.lockout
			}
		}
	}
	return retryAfter, retryAfter > 0
}

func (l *loginRateLimiter) recordSuccess(ip string, username string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	buckets := loginBuckets(ip, username)
	// A valid login clears failures for this account and exact source/account
	// pair. It does not make unrelated username failures from the same source
	// benign, so retain the broad IP bucket used to stop username floods.
	for _, bucket := range buckets[1:] {
		attempt, ok := l.attempts[bucket.key]
		if !ok {
			continue
		}
		attempt.failures = 0
		attempt.firstFail = time.Time{}
		attempt.lockedUntil = time.Time{}
		attempt.lockAuditAt = time.Time{}
		l.touch(attempt)
	}
}

// claimLockoutAudit permits one immediate lockout audit and then one sample per
// period. It uses the broadest active bucket first, so random usernames behind
// a locked IP and distributed IPs attacking one account share the same bound.
func (l *loginRateLimiter) claimLockoutAudit(ip string, username string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	for _, bucket := range loginBuckets(ip, username) {
		attempt, ok := l.attempts[bucket.key]
		if !ok || !now.Before(attempt.lockedUntil) {
			continue
		}
		if attempt.lockAuditAt.IsZero() || now.Sub(attempt.lockAuditAt) >= loginLockoutAuditPeriod {
			attempt.lockAuditAt = now
			return true
		}
		return false
	}
	return false
}

func (l *loginRateLimiter) ensureBuckets(buckets [3]loginBucket, now time.Time) bool {
	missing := 0
	for _, bucket := range buckets {
		if _, ok := l.attempts[bucket.key]; !ok {
			missing++
		}
	}
	for element := l.recency.Front(); len(l.attempts)+missing > l.maxEntries && element != nil; {
		next := element.Next()
		key := element.Value.(string)
		if !l.active(l.attempts[key], now) {
			l.delete(key)
		}
		element = next
	}
	if len(l.attempts)+missing > l.maxEntries {
		return false
	}
	for _, bucket := range buckets {
		if _, ok := l.attempts[bucket.key]; ok {
			continue
		}
		element := l.recency.PushBack(bucket.key)
		l.attempts[bucket.key] = &loginAttempt{recency: element}
	}
	return true
}

func (l *loginRateLimiter) active(attempt *loginAttempt, now time.Time) bool {
	if attempt == nil || attempt.inFlight > 0 || now.Before(attempt.lockedUntil) {
		return attempt != nil
	}
	return attempt.failures > 0 && !attempt.firstFail.IsZero() && now.Sub(attempt.firstFail) <= l.window
}

func (l *loginRateLimiter) touch(attempt *loginAttempt) {
	l.recency.MoveToBack(attempt.recency)
}

func (l *loginRateLimiter) delete(key string) {
	attempt, ok := l.attempts[key]
	if !ok {
		return
	}
	l.recency.Remove(attempt.recency)
	delete(l.attempts, key)
}

func loginBuckets(ip string, username string) [3]loginBucket {
	normalizedUsername := strings.ToLower(strings.TrimSpace(username))
	digest := sha256.Sum256([]byte(normalizedUsername))
	account := hex.EncodeToString(digest[:])
	return [3]loginBucket{
		{key: "ip:" + ip, maxFailures: loginIPMaxFailures},
		{key: "account:" + account, maxFailures: loginAccountMaxFailures},
		{key: "pair:" + ip + ":" + account, maxFailures: loginPairMaxFailures},
	}
}
