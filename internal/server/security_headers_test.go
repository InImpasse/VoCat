package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSecurityHeadersSetsHSTSOnlyForVerifiedHTTPS(t *testing.T) {
	trustedProxy, err := parseAccessConfig(accessConfig{
		Mode:              "internal",
		TrustProxyHeaders: true,
		TrustedProxyCIDRs: []string{"203.0.113.10/32"},
	})
	if err != nil {
		t.Fatal(err)
	}
	legacyProxyTrust := parsedAccessConfig{mode: "internal", trustProxy: true}

	for _, test := range []struct {
		name       string
		url        string
		remoteAddr string
		config     parsedAccessConfig
		protos     []string
		wantHSTS   bool
	}{
		{
			name:       "direct TLS",
			url:        "https://vocat.example/",
			remoteAddr: "198.51.100.20:5000",
			protos:     []string{"http, https"},
			wantHSTS:   true,
		},
		{
			name:       "trusted proxy HTTPS",
			url:        "http://vocat.example/",
			remoteAddr: "203.0.113.10:5000",
			config:     trustedProxy,
			protos:     []string{"https"},
			wantHSTS:   true,
		},
		{
			name:       "plain HTTP",
			url:        "http://vocat.example/",
			remoteAddr: "198.51.100.20:5000",
		},
		{
			name:       "untrusted proxy header",
			url:        "http://vocat.example/",
			remoteAddr: "198.51.100.20:5000",
			config:     trustedProxy,
			protos:     []string{"https"},
		},
		{
			name:       "legacy trust without CIDR",
			url:        "http://vocat.example/",
			remoteAddr: "203.0.113.10:5000",
			config:     legacyProxyTrust,
			protos:     []string{"https"},
		},
		{
			name:       "trusted proxy HTTP",
			url:        "http://vocat.example/",
			remoteAddr: "203.0.113.10:5000",
			config:     trustedProxy,
			protos:     []string{"http"},
		},
		{
			name:       "mixed forwarded proto",
			url:        "http://vocat.example/",
			remoteAddr: "203.0.113.10:5000",
			config:     trustedProxy,
			protos:     []string{"https, http"},
		},
		{
			name:       "repeated forwarded proto",
			url:        "http://vocat.example/",
			remoteAddr: "203.0.113.10:5000",
			config:     trustedProxy,
			protos:     []string{"https", "https"},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := &Server{access: test.config, secureCookies: true}
			handler := server.securityHeaders(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNoContent)
			}))
			request := httptest.NewRequest(http.MethodGet, test.url, nil)
			request.RemoteAddr = test.remoteAddr
			for _, proto := range test.protos {
				request.Header.Add("X-Forwarded-Proto", proto)
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			gotHSTS := response.Header().Get("Strict-Transport-Security") != ""
			if gotHSTS != test.wantHSTS {
				t.Fatalf("HSTS present = %t, want %t", gotHSTS, test.wantHSTS)
			}
		})
	}
}
