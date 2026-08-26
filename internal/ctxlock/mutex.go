package ctxlock

import (
	"context"
	"sync"
)

// Mutex is a zero-value-ready mutex whose request paths can be canceled.
type Mutex struct {
	once  sync.Once
	token chan struct{}
}

func (mutex *Mutex) initialize() {
	mutex.once.Do(func() {
		mutex.token = make(chan struct{}, 1)
		mutex.token <- struct{}{}
	})
}

func (mutex *Mutex) Lock() {
	_ = mutex.LockContext(context.Background())
}

func (mutex *Mutex) LockContext(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	mutex.initialize()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-mutex.token:
		return nil
	}
}

func (mutex *Mutex) Unlock() {
	mutex.initialize()
	select {
	case mutex.token <- struct{}{}:
	default:
		panic("ctxlock: unlock of unlocked mutex")
	}
}
