package api

import (
	"log"
	"net"
	"net/http"
	"sync"
	"time"
)

// denyTrace rejects methods that must never be served (A02). Go's mux routes
// by path regardless of method token, so TRACE would otherwise execute the
// handler — fail closed instead.
func denyTrace(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodTrace, "TRACK", http.MethodConnect:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// securityHeaders sets the A04/A08 baseline on every response. The API speaks
// JSON (and one WS endpoint) — a restrictive posture costs nothing here.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		next.ServeHTTP(w, r)
	})
}

// RateLimiter is a fixed-window per-IP limiter for abuse-relevant endpoints
// (A06: ingest is the spam/resource-exhaustion vector — unbounded POSTs become
// webhook deliveries). Fail closed on accounting errors is unnecessary here:
// the limiter is best-effort in-memory; the security property is that bursts
// get 429s, not precise accounting.
type RateLimiter struct {
	mu     sync.Mutex
	hits   map[string]*window
	Limit  int
	Window time.Duration
}

type window struct {
	count int
	reset time.Time
}

// NewRateLimiter allows limit requests per window per client IP.
func NewRateLimiter(limit int, per time.Duration) *RateLimiter {
	return &RateLimiter{hits: map[string]*window{}, Limit: limit, Window: per}
}

func clientIP(r *http.Request) string {
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

// Allow reports whether the request may proceed.
func (l *RateLimiter) Allow(r *http.Request) (bool, time.Duration) {
	if l == nil || l.Limit <= 0 {
		return true, 0
	}
	ip := clientIP(r)
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()
	w, ok := l.hits[ip]
	if !ok || now.After(w.reset) {
		l.hits[ip] = &window{count: 1, reset: now.Add(l.Window)}
		return true, 0
	}
	w.count++
	if w.count > l.Limit {
		return false, time.Until(w.reset)
	}
	return true, 0
}

func (l *RateLimiter) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ok, retry := l.Allow(r)
		if !ok {
			secs := int(retry.Seconds()) + 1
			w.Header().Set("Retry-After", itoa(secs))
			http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [8]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

// internalError logs the detail server-side and returns a generic body —
// error responses must never leak drivers, tables, or dial addresses (A10).
func internalError(w http.ResponseWriter, err error) {
	log.Printf("api: internal error (detail server-side only): %v", err)
	http.Error(w, "internal error", http.StatusInternalServerError)
}
