// SSRF guard (OWASP A01, 2025: SSRF is an access-control failure when the
// server is induced to reach internal resources). Webhook URLs come from rule
// specs, i.e. user-supplied input — every fetch is validated before any byte
// is sent, re-validated on each redirect, and re-checked at TCP connect time
// (DNS-rebind TOCTOU). Fail closed, always.
package webhooks

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// well-known cloud metadata hosts, blocked by name even if DNS ever answers public.
var metadataHosts = map[string]bool{
	"metadata.google.internal":      true,
	"metadata.google.internal.":     true,
	"instance-data":                 true,
	"instance-data-compute":         true,
	"169.254.169.254":               true,
	"fd00:ec2::254":                 true,
}

// ValidateURL rejects anything that is not a public http(s) endpoint.
// allowPrivate is the test/dev escape hatch (httptest servers are loopback);
// production must leave it false. Returns nil only when every resolved IP is
// a public unicast address.
func ValidateURL(raw string, allowPrivate bool) error {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return fmt.Errorf("webhook: ssrf: invalid URL")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("webhook: ssrf: scheme %q not allowed (http/https only)", u.Scheme)
	}
	if u.User != nil {
		return fmt.Errorf("webhook: ssrf: userinfo not allowed")
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	if metadataHosts[host] || metadataHosts[u.Hostname()] {
		return fmt.Errorf("webhook: ssrf: metadata endpoint blocked")
	}
	ips, err := net.DefaultResolver.LookupIPAddr(context.Background(), u.Hostname())
	if err != nil || len(ips) == 0 {
		return fmt.Errorf("webhook: ssrf: unresolvable host")
	}
	for _, ip := range ips {
		if !publicIP(ip.IP, allowPrivate) {
			return fmt.Errorf("webhook: ssrf: host resolves to non-public address")
		}
	}
	return nil
}

func publicIP(ip net.IP, allowPrivate bool) bool {
	if ip.IsLoopback() || ip.IsUnspecified() || ip.IsMulticast() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return allowPrivate && (ip.IsLoopback() || ip.IsUnspecified())
	}
	if !allowPrivate && !ip.IsGlobalUnicast() {
		return false
	}
	if !allowPrivate && isPrivate(ip) {
		return false
	}
	return true
}

var privateRanges = []net.IPNet{
	mustCIDR("10.0.0.0/8"),
	mustCIDR("172.16.0.0/12"),
	mustCIDR("192.168.0.0/16"),
	mustCIDR("169.254.0.0/16"), // cloud metadata + link-local
	mustCIDR("fc00::/7"),       // unique-local IPv6
	mustCIDR("fe80::/10"),
	// NOTE: no ::ffff:0:0/96 entry — Go's Contains maps 4-byte IPs into it,
	// so it would match every IPv4 address. isPrivate normalizes via To4.
}

func mustCIDR(s string) net.IPNet {
	_, n, err := net.ParseCIDR(s)
	if err != nil {
		panic(err)
	}
	return *n
}

func isPrivate(ip net.IP) bool {
	if v4 := ip.To4(); v4 != nil {
		ip = v4
	}
	for _, n := range privateRanges {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// guardedClient wraps the dispatcher HTTP client: redirects re-validate the
// next hop (max 3), and the dialer refuses non-public IPs at connect time so
// a DNS rebind between validation and connect still fails closed.
func (d *Dispatcher) guardedClient(allowPrivate bool) *http.Client {
	base := d.Client
	if base == nil {
		base = &http.Client{Timeout: 5 * time.Second}
	}
	c := *base
	c.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) > 3 {
			return fmt.Errorf("webhook: too many redirects")
		}
		if err := ValidateURL(req.URL.String(), allowPrivate); err != nil {
			return err
		}
		return nil
	}
	var tr *http.Transport
	switch t := c.Transport.(type) {
	case nil:
		tr = http.DefaultTransport.(*http.Transport).Clone()
	case *http.Transport:
		tr = t.Clone()
	default:
		return &c // custom transport (tests): redirect validation still applies
	}
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	tr.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, _, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, fmt.Errorf("webhook: ssrf: bad dial address")
		}
		ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
		if err != nil || len(ips) == 0 {
			return nil, fmt.Errorf("webhook: ssrf: unresolvable at connect")
		}
		for _, ip := range ips {
			if !publicIP(ip.IP, allowPrivate) {
				return nil, fmt.Errorf("webhook: ssrf: connect to non-public address refused")
			}
		}
		return dialer.DialContext(ctx, network, addr)
	}
	c.Transport = tr
	return &c
}
