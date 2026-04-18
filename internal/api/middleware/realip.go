package middleware

import (
	"net"
	"net/http"
	"os"
	"strings"
)

// trustedProxyCIDRs is the list of subnets whose X-Forwarded-For / X-Real-IP
// headers are trusted. Defaults to Docker's default bridge network and
// loopback; operators can override via the TRUSTED_PROXY_CIDRS env var
// (comma-separated, e.g. "10.0.0.0/8,192.168.0.0/16").
var trustedProxyCIDRs = loadTrustedProxyCIDRs()

func loadTrustedProxyCIDRs() []*net.IPNet {
	raw := os.Getenv("TRUSTED_PROXY_CIDRS")
	if strings.TrimSpace(raw) == "" {
		raw = "127.0.0.0/8,::1/128,172.16.0.0/12,10.0.0.0/8,192.168.0.0/16"
	}
	var out []*net.IPNet
	for _, cidr := range strings.Split(raw, ",") {
		cidr = strings.TrimSpace(cidr)
		if cidr == "" {
			continue
		}
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			continue
		}
		out = append(out, network)
	}
	return out
}

func isTrustedProxy(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	for _, network := range trustedProxyCIDRs {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

// TrustedProxyRealIP replaces r.RemoteAddr with the client IP reported by
// X-Forwarded-For / X-Real-IP, but ONLY when the immediate peer is in a
// trusted proxy CIDR. This prevents spoofing of rate-limit identifiers
// from untrusted clients.
//
// For the leftmost X-Forwarded-For value, the header is trusted because the
// reverse proxy (nginx-proxy) appends the real client IP to a possibly
// attacker-supplied list. The leftmost entry is the originating client IP
// as reported by the first trusted hop.
func TrustedProxyRealIP() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !isTrustedProxy(r.RemoteAddr) {
				// Untrusted peer; ignore any proxy headers.
				next.ServeHTTP(w, r)
				return
			}

			if xri := strings.TrimSpace(r.Header.Get("X-Real-IP")); xri != "" {
				r.RemoteAddr = xri
				next.ServeHTTP(w, r)
				return
			}

			if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
				// The leftmost entry in X-Forwarded-For is the originating client.
				// We trust it only because the immediate peer is a trusted proxy.
				parts := strings.Split(xff, ",")
				if first := strings.TrimSpace(parts[0]); first != "" {
					r.RemoteAddr = first
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}
