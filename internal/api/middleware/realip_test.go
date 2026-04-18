package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTrustedProxyRealIP(t *testing.T) {
	tests := []struct {
		name       string
		remoteAddr string
		headers    map[string]string
		want       string
	}{
		{
			name:       "untrusted peer with XFF is ignored",
			remoteAddr: "203.0.113.5:44321",
			headers:    map[string]string{"X-Forwarded-For": "1.2.3.4"},
			want:       "203.0.113.5:44321",
		},
		{
			name:       "untrusted peer with X-Real-IP is ignored",
			remoteAddr: "203.0.113.5:44321",
			headers:    map[string]string{"X-Real-IP": "1.2.3.4"},
			want:       "203.0.113.5:44321",
		},
		{
			name:       "trusted docker-bridge peer honors X-Real-IP",
			remoteAddr: "172.18.0.4:56000",
			headers:    map[string]string{"X-Real-IP": "198.51.100.7"},
			want:       "198.51.100.7",
		},
		{
			name:       "trusted peer honors leftmost XFF entry",
			remoteAddr: "172.18.0.4:56000",
			headers:    map[string]string{"X-Forwarded-For": "198.51.100.7, 10.0.0.1"},
			want:       "198.51.100.7",
		},
		{
			name:       "loopback peer is trusted",
			remoteAddr: "127.0.0.1:40000",
			headers:    map[string]string{"X-Forwarded-For": "8.8.8.8"},
			want:       "8.8.8.8",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var got string
			handler := TrustedProxyRealIP()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				got = r.RemoteAddr
			}))
			req := httptest.NewRequest("GET", "/", nil)
			req.RemoteAddr = tc.remoteAddr
			for k, v := range tc.headers {
				req.Header.Set(k, v)
			}
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)
			if got != tc.want {
				t.Errorf("RemoteAddr = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestRateLimiterIgnoresSpoofedXFFFromUntrustedClient(t *testing.T) {
	// An attacker behind no proxy cycles X-Forwarded-For; TrustedProxyRealIP
	// must not rewrite RemoteAddr, so the rate limiter sees the same IP each time.
	limiter := NewRateLimiter(2, 60*60*1e9) // 2 per hour

	pipeline := TrustedProxyRealIP()(limiter.Middleware("oauth_start")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})))

	for i, spoofed := range []string{"1.1.1.1", "2.2.2.2", "3.3.3.3"} {
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = "203.0.113.5:40000" // untrusted peer — does NOT change across requests
		req.Header.Set("X-Forwarded-For", spoofed)
		rr := httptest.NewRecorder()
		pipeline.ServeHTTP(rr, req)
		if i < 2 && rr.Code != http.StatusOK {
			t.Fatalf("request %d: expected 200, got %d", i, rr.Code)
		}
		if i == 2 && rr.Code != http.StatusTooManyRequests {
			t.Fatalf("request %d: expected 429 (spoofing should not reset bucket), got %d", i, rr.Code)
		}
	}
}
