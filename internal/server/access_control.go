package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/netip"
	"strings"

	"vocat/internal/store"
)

const accessSettingKey = "security.access"

// accessConfig is the persisted network access policy.
type accessConfig struct {
	Mode              string   `json:"mode"`                // "internal" (default) or "public"
	AllowedCIDRs      []string `json:"allowed_cidrs"`       // extra CIDRs always allowed
	TrustProxyHeaders bool     `json:"trust_proxy_headers"` // honor headers only from trusted proxies
	TrustedProxyCIDRs []string `json:"trusted_proxy_cidrs"` // reverse proxies allowed to supply client IPs
}

// parsedAccessConfig is the validated runtime form of accessConfig.
type parsedAccessConfig struct {
	mode              string
	cidrs             []netip.Prefix
	trustProxy        bool
	trustedProxyCIDRs []netip.Prefix
}

// internalNetworks are always allowed when mode is "internal": loopback,
// RFC1918 private ranges, Tailscale IPv4, link-local, and IPv6 ULA.
var internalNetworks = []netip.Prefix{
	netip.MustParsePrefix("127.0.0.0/8"),
	netip.MustParsePrefix("10.0.0.0/8"),
	netip.MustParsePrefix("172.16.0.0/12"),
	netip.MustParsePrefix("192.168.0.0/16"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("169.254.0.0/16"),
	netip.MustParsePrefix("::1/128"),
	netip.MustParsePrefix("fe80::/10"),
	netip.MustParsePrefix("fc00::/7"),
}

func defaultAccessConfig() parsedAccessConfig {
	return parsedAccessConfig{mode: "internal"}
}

// parseAccessConfig validates and parses a persisted access policy.
func parseAccessConfig(config accessConfig) (parsedAccessConfig, error) {
	mode := strings.ToLower(strings.TrimSpace(config.Mode))
	if mode == "" {
		mode = "internal"
	}
	if mode != "internal" && mode != "public" {
		return parsedAccessConfig{}, errors.New("mode must be \"internal\" or \"public\"")
	}
	parsed := parsedAccessConfig{
		mode:       mode,
		trustProxy: config.TrustProxyHeaders,
	}
	var err error
	parsed.cidrs, err = parseAccessPrefixes(config.AllowedCIDRs)
	if err != nil {
		return parsedAccessConfig{}, err
	}
	parsed.trustedProxyCIDRs, err = parseAccessPrefixes(config.TrustedProxyCIDRs)
	if err != nil {
		return parsedAccessConfig{}, errors.New("invalid trusted proxy " + err.Error())
	}
	return parsed, nil
}

func parseAccessPrefixes(values []string) ([]netip.Prefix, error) {
	prefixes := make([]netip.Prefix, 0, len(values))
	for _, raw := range values {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		if prefix, err := netip.ParsePrefix(raw); err == nil {
			address := prefix.Addr()
			bits := prefix.Bits()
			if address.Is4In6() && bits >= 96 {
				address = address.Unmap()
				bits -= 96
			}
			prefixes = append(prefixes, netip.PrefixFrom(address.WithZone(""), bits).Masked())
			continue
		}
		if address, err := netip.ParseAddr(raw); err == nil {
			address = address.Unmap().WithZone("")
			bits := 32
			if address.Is6() {
				bits = 128
			}
			prefixes = append(prefixes, netip.PrefixFrom(address, bits))
			continue
		}
		return nil, errors.New("invalid CIDR or IP: " + raw)
	}
	return prefixes, nil
}

// allowed reports whether a client address may reach the service.
func (config parsedAccessConfig) allowed(address netip.Addr) bool {
	if !address.IsValid() {
		return false
	}
	// Normalize IPv4-mapped IPv6 addresses (e.g. ::ffff:192.168.1.5 seen on
	// dual-stack listeners) to their IPv4 form so they match the internal
	// ranges below; without this they would be denied even though they are
	// ordinary internal IPv4 clients.
	address = address.Unmap()
	if config.mode == "public" {
		return true
	}
	if address.IsLoopback() {
		return true
	}
	for _, prefix := range internalNetworks {
		if prefix.Contains(address) {
			return true
		}
	}
	for _, prefix := range config.cidrs {
		if prefix.Contains(address) {
			return true
		}
	}
	return false
}

// clientIP determines the request's source address. Forwarded headers are used
// only when the direct TCP peer belongs to an explicitly trusted proxy range.
func (config parsedAccessConfig) clientIP(r *http.Request) netip.Addr {
	peer := directPeerIP(r.RemoteAddr)
	if !peer.IsValid() || !config.trustProxy || len(config.trustedProxyCIDRs) == 0 || !config.isTrustedProxy(peer) {
		return peer
	}

	forwardedValues := r.Header.Values("X-Forwarded-For")
	if len(forwardedValues) > 0 {
		forwarded := strings.TrimSpace(strings.Join(forwardedValues, ","))
		chain, ok := parseForwardedChain(forwarded)
		if !ok {
			return netip.Addr{}
		}
		chain = append(chain, peer)
		for index := len(chain) - 1; index >= 0; index-- {
			if !config.isTrustedProxy(chain[index]) {
				return chain[index]
			}
		}
		return chain[0]
	}
	realValues := r.Header.Values("X-Real-IP")
	if len(realValues) > 0 {
		if len(realValues) != 1 {
			return netip.Addr{}
		}
		real := strings.TrimSpace(realValues[0])
		address, err := netip.ParseAddr(real)
		if err != nil {
			return netip.Addr{}
		}
		return address.Unmap().WithZone("")
	}
	return peer
}

func directPeerIP(remoteAddr string) netip.Addr {
	addressPort, err := netip.ParseAddrPort(strings.TrimSpace(remoteAddr))
	if err != nil {
		return netip.Addr{}
	}
	return addressPort.Addr().Unmap().WithZone("")
}

func parseForwardedChain(forwarded string) ([]netip.Addr, bool) {
	parts := strings.Split(forwarded, ",")
	chain := make([]netip.Addr, 0, len(parts))
	for _, part := range parts {
		address, err := netip.ParseAddr(strings.TrimSpace(part))
		if err != nil {
			return nil, false
		}
		chain = append(chain, address.Unmap().WithZone(""))
	}
	return chain, len(chain) > 0
}

func (config parsedAccessConfig) isTrustedProxy(address netip.Addr) bool {
	for _, prefix := range config.trustedProxyCIDRs {
		if prefix.Contains(address) {
			return true
		}
	}
	return false
}

// accessControl rejects requests whose source IP is outside the configured
// access policy. It wraps the whole mux so every route (API, SPA, websheets) is
// protected uniformly.
func (s *Server) accessControl(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.accessMu.RLock()
		config := s.access
		s.accessMu.RUnlock()
		address := config.clientIP(r)
		if config.allowed(address) {
			next.ServeHTTP(w, r)
			return
		}
		s.logger.Warn(
			"request denied by network access policy",
			"remote_addr", r.RemoteAddr,
			"client_ip", address.String(),
			"path", r.URL.Path,
		)
		writeError(
			w,
			http.StatusForbidden,
			"network_access_denied",
			"access is restricted to internal network addresses",
		)
	})
}

func (s *Server) currentAccessConfig() parsedAccessConfig {
	s.accessMu.RLock()
	defer s.accessMu.RUnlock()
	return s.access
}

func (config parsedAccessConfig) persisted() accessConfig {
	return accessConfig{
		Mode:              config.mode,
		AllowedCIDRs:      prefixStrings(config.cidrs),
		TrustProxyHeaders: config.trustProxy,
		TrustedProxyCIDRs: prefixStrings(config.trustedProxyCIDRs),
	}
}

func (config parsedAccessConfig) response(address netip.Addr) map[string]any {
	return map[string]any{
		"mode":                config.mode,
		"allowed_cidrs":       prefixStrings(config.cidrs),
		"trust_proxy_headers": config.trustProxy,
		"trusted_proxy_cidrs": prefixStrings(config.trustedProxyCIDRs),
		"client_ip":           address.String(),
		"client_allowed":      config.allowed(address),
	}
}

func prefixStrings(prefixes []netip.Prefix) []string {
	values := make([]string, 0, len(prefixes))
	for _, prefix := range prefixes {
		values = append(values, prefix.String())
	}
	return values
}

// loadAccessConfig reads the persisted policy (defaulting to internal) into the
// runtime cache. Called at startup.
func (s *Server) loadAccessConfig(ctx context.Context) {
	config := defaultAccessConfig()
	setting, err := s.store.AppSetting(ctx, accessSettingKey)
	if err == nil {
		var stored accessConfig
		if json.Unmarshal(setting.Value, &stored) == nil {
			if parsed, parseErr := parseAccessConfig(stored); parseErr == nil {
				config = parsed
			}
		}
	} else if !errors.Is(err, store.ErrNotFound) {
		s.logger.Warn("load access policy failed", "error", err)
	}
	s.accessMu.Lock()
	s.access = config
	s.accessMu.Unlock()
}

// handleSecuritySettings reads and writes the network access policy.
//
//	GET /api/settings/security
//	PUT /api/settings/security
func (s *Server) handleSecuritySettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		config := s.currentAccessConfig()
		address := config.clientIP(r)
		writeJSON(w, http.StatusOK, map[string]any{
			"data": config.response(address),
		})
	case http.MethodPut:
		var request accessConfig
		if err := s.decodeJSON(w, r, &request); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}
		parsed, err := parseAccessConfig(request)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_access_policy", err.Error())
			return
		}
		payload, err := json.Marshal(parsed.persisted())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "an internal error occurred")
			return
		}
		if err := s.store.UpsertAppSetting(r.Context(), store.AppSetting{
			Key:   accessSettingKey,
			Value: payload,
		}); err != nil {
			s.writeStoreError(w, err)
			return
		}
		s.accessMu.Lock()
		s.access = parsed
		s.accessMu.Unlock()
		s.audit(r, "settings.security.update", "settings", "security", "success")
		address := parsed.clientIP(r)
		writeJSON(w, http.StatusOK, map[string]any{
			"data": parsed.response(address),
		})
	default:
		w.Header().Set("Allow", "GET, PUT")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
	}
}
