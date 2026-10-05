package ssrf

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var (
	// ErrBlockedAddress is returned when a target IP address falls into a forbidden or private range.
	ErrBlockedAddress = errors.New("destination IP address is blocked by SSRF defense policy")
	// ErrInvalidScheme is returned when a URL uses a scheme other than HTTP or HTTPS.
	ErrInvalidScheme = errors.New("unsupported protocol scheme, only HTTP and HTTPS are permitted")
	// ErrEmptyHost is returned when a URL does not specify a hostname.
	ErrEmptyHost = errors.New("target host is empty")
	// ErrRedirectLimitExceeded is returned when too many HTTP redirects occur.
	ErrRedirectLimitExceeded = errors.New("exceeded maximum redirect limit of 5 hops")
)

// List of reserved CIDR blocks that must never be accessed via user-supplied URLs.
var blockedCIDRs []*net.IPNet

func init() {
	cidrs := []string{
		// IPv4 Private & Special
		"0.0.0.0/8",          // "This" Network
		"10.0.0.0/8",         // Private RFC 1918
		"100.64.0.0/10",      // Shared Address Space / CGNAT RFC 6598
		"127.0.0.0/8",        // Loopback
		"169.254.0.0/16",     // Link Local & Cloud Metadata (169.254.169.254)
		"172.16.0.0/12",      // Private RFC 1918
		"192.0.0.0/24",       // IETF Protocol Assignments RFC 6890
		"192.0.2.0/24",       // TEST-NET-1 Documentation
		"192.168.0.0/16",     // Private RFC 1918
		"198.18.0.0/15",      // Network Benchmark Tests RFC 2544
		"198.51.100.0/24",    // TEST-NET-2 Documentation
		"203.0.113.0/24",     // TEST-NET-3 Documentation
		"224.0.0.0/4",        // Multicast
		"240.0.0.0/4",        // Reserved for Future Use
		"255.255.255.255/32", // Limited Broadcast

		// IPv6 Special
		"::/128",        // Unspecified
		"::1/128",       // Loopback
		"100::/64",      // Discard-Only RFC 6666
		"2001:db8::/32", // Documentation
		"fc00::/7",      // Unique Local Address (ULA)
		"fe80::/10",     // Link-Local Unicast
		"ff00::/8",      // Multicast
	}

	for _, cidr := range cidrs {
		_, ipNet, err := net.ParseCIDR(cidr)
		if err == nil && ipNet != nil {
			blockedCIDRs = append(blockedCIDRs, ipNet)
		}
	}
}

// IsBlockedIP verifies if an IP address belongs to loopback, private, link-local, multicast, or reserved ranges.
func IsBlockedIP(ip net.IP) bool {
	if ip == nil {
		return true
	}

	// If it is an IPv4 address (even if represented as 16-byte IPv4-mapped IPv6), canonicalize to 4-byte IPv4
	if ipv4 := ip.To4(); ipv4 != nil {
		ip = ipv4
	}

	// Check standard library classifications
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() {
		return true
	}

	// Check explicit reserved CIDR ranges
	for _, block := range blockedCIDRs {
		if block.Contains(ip) {
			return true
		}
	}

	return false
}

// ValidateURL performs static and DNS pre-flight verification on a target URL string.
func ValidateURL(ctx context.Context, rawURL string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return nil, fmt.Errorf("invalid URL format: %w", err)
	}

	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return nil, ErrInvalidScheme
	}

	host := parsed.Hostname()
	if host == "" {
		return nil, ErrEmptyHost
	}

	lowerHost := strings.ToLower(host)
	if lowerHost == "localhost" ||
		strings.HasSuffix(lowerHost, ".local") ||
		strings.HasSuffix(lowerHost, ".internal") ||
		strings.HasSuffix(lowerHost, ".localhost") {
		return nil, ErrBlockedAddress
	}

	// If host is a raw IP literal
	if ip := net.ParseIP(host); ip != nil {
		if IsBlockedIP(ip) {
			return nil, ErrBlockedAddress
		}
		return parsed, nil
	}

	// Resolve hostname to IP addresses and inspect each
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve host '%s': %w", host, err)
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("no IP addresses resolved for host '%s'", host)
	}

	for _, addr := range ips {
		if IsBlockedIP(addr.IP) {
			return nil, ErrBlockedAddress
		}
	}

	return parsed, nil
}

// NewSafeTransport returns an http.Transport with DNS-recheck dial controls to prevent DNS rebinding attacks.
func NewSafeTransport(timeout time.Duration) *http.Transport {
	dialer := &net.Dialer{
		Timeout:   timeout,
		KeepAlive: 30 * time.Second,
	}

	return &http.Transport{
		Proxy:                 nil, // Prevent local environment proxy interception
		MaxIdleConns:          50,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}

			// Validate IP if host is an IP literal
			if ip := net.ParseIP(host); ip != nil {
				if IsBlockedIP(ip) {
					return nil, ErrBlockedAddress
				}
				return dialer.DialContext(ctx, network, addr)
			}

			// Resolve host to ensure zero DNS-rebinding / TOCTOU vulnerability
			ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
			if err != nil {
				return nil, fmt.Errorf("dns lookup failed for '%s': %w", host, err)
			}
			if len(ips) == 0 {
				return nil, fmt.Errorf("no IP address found for '%s'", host)
			}

			var dialErr error
			for _, ipAddr := range ips {
				if IsBlockedIP(ipAddr.IP) {
					return nil, ErrBlockedAddress
				}

				targetAddr := net.JoinHostPort(ipAddr.IP.String(), port)
				conn, err := dialer.DialContext(ctx, network, targetAddr)
				if err == nil {
					return conn, nil
				}
				dialErr = err
			}

			if dialErr != nil {
				return nil, dialErr
			}
			return nil, errors.New("unable to establish connection to resolved destination")
		},
	}
}

// NewSafeClient returns an *http.Client configured with Anti-SSRF transport and redirect guards.
func NewSafeClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Transport: NewSafeTransport(timeout),
		Timeout:   timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return ErrRedirectLimitExceeded
			}

			// Validate redirect destination scheme
			scheme := strings.ToLower(req.URL.Scheme)
			if scheme != "http" && scheme != "https" {
				return ErrInvalidScheme
			}

			// Static pre-check on host
			host := req.URL.Hostname()
			if host == "" {
				return ErrEmptyHost
			}
			lowerHost := strings.ToLower(host)
			if lowerHost == "localhost" ||
				strings.HasSuffix(lowerHost, ".local") ||
				strings.HasSuffix(lowerHost, ".internal") ||
				strings.HasSuffix(lowerHost, ".localhost") {
				return ErrBlockedAddress
			}

			if ip := net.ParseIP(host); ip != nil {
				if IsBlockedIP(ip) {
					return ErrBlockedAddress
				}
			}

			return nil
		},
	}
}

// FetchMediaStream securely executes an HTTP GET request to targetURL after passing Anti-SSRF verification.
// The caller is responsible for closing the returned Response.Body.
func FetchMediaStream(ctx context.Context, targetURL string, timeout time.Duration) (*http.Response, error) {
	if _, err := ValidateURL(ctx, targetURL); err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to construct http request: %w", err)
	}

	req.Header.Set("User-Agent", "Youten-AI-MediaFetcher/1.0")

	client := NewSafeClient(timeout)
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute safe fetch: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("upstream server returned non-200 status code: %d", resp.StatusCode)
	}

	return resp, nil
}

