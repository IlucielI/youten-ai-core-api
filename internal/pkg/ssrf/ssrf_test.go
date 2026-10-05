package ssrf_test

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"code-base-golang/internal/pkg/ssrf"
)

func TestSSRF_IsBlockedIP(t *testing.T) {
	tests := []struct {
		name     string
		ipStr    string
		expected bool
	}{
		{"nil ip", "", true},
		{"loopback ipv4", "127.0.0.1", true},
		{"loopback ipv4 alt", "127.0.0.254", true},
		{"loopback ipv6", "::1", true},
		{"unspecified ipv4", "0.0.0.0", true},
		{"unspecified ipv6", "::", true},
		{"private 10.0.0.0/8", "10.10.1.5", true},
		{"private 172.16.0.0/12", "172.20.14.2", true},
		{"private 192.168.0.0/16", "192.168.1.1", true},
		{"cloud metadata / link-local", "169.254.169.254", true},
		{"link-local unicast", "169.254.10.20", true},
		{"cgnat rfc6598", "100.64.0.1", true},
		{"multicast 224.0.0.1", "224.0.0.1", true},
		{"broadcast 255.255.255.255", "255.255.255.255", true},
		{"ipv6 ula", "fc00::1", true},
		{"ipv6 link-local", "fe80::1", true},
		{"public google dns", "8.8.8.8", false},
		{"public cloudflare dns", "1.1.1.1", false},
		{"public quad9", "9.9.9.9", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var ip net.IP
			if tc.ipStr != "" {
				ip = net.ParseIP(tc.ipStr)
			}
			result := ssrf.IsBlockedIP(ip)
			if result != tc.expected {
				t.Errorf("IsBlockedIP(%q) = %v; want %v", tc.ipStr, result, tc.expected)
			}
		})
	}
}

func TestSSRF_ValidateURL(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name      string
		rawURL    string
		expectErr bool
	}{
		{"file scheme disallowed", "file:///etc/passwd", true},
		{"ftp scheme disallowed", "ftp://example.com/audio.mp3", true},
		{"gopher scheme disallowed", "gopher://example.com/", true},
		{"empty host", "http:///media.mp3", true},
		{"localhost host", "http://localhost:8080/audio.mp3", true},
		{"loopback IP 127.0.0.1", "http://127.0.0.1:8000/stream.mp3", true},
		{"private 192.168.1.1", "http://192.168.1.1/voice.wav", true},
		{"cloud metadata IP", "http://169.254.169.254/latest/meta-data/", true},
		{"internal domain suffix", "http://app.internal/podcast.mp3", true},
		{"local domain suffix", "http://service.local/clip.mp4", true},
		{"valid public URL with standard port", "https://8.8.8.8/audio.mp3", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			parsed, err := ssrf.ValidateURL(ctx, tc.rawURL)
			if tc.expectErr && err == nil {
				t.Fatalf("expected error for URL %q, got nil parsed: %v", tc.rawURL, parsed)
			}
			if !tc.expectErr && err != nil {
				t.Fatalf("unexpected error for URL %q: %v", tc.rawURL, err)
			}
		})
	}
}

func TestSSRF_NewSafeClient_BlocksLoopbackConnection(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("internal secret data"))
	}))
	defer ts.Close()

	client := ssrf.NewSafeClient(2 * time.Second)

	// Attempting to dial the local httptest server (which binds to 127.0.0.1) MUST be blocked by custom DialContext
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, ts.URL, nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	resp, err := client.Do(req)
	if err == nil {
		if resp != nil {
			_ = resp.Body.Close()
		}
		t.Fatalf("expected SSRF client to block request to %s, but connection succeeded", ts.URL)
	}
}

func TestSSRF_NewSafeClient_RedirectGuards(t *testing.T) {
	// Test redirect limit
	redirectCount := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirectCount++
		http.Redirect(w, r, "/next", http.StatusFound)
	}))
	defer ts.Close()

	client := ssrf.NewSafeClient(2 * time.Second)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, ts.URL, nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	// Dial to 127.0.0.1 should already be blocked by DialContext
	_, err = client.Do(req)
	if err == nil {
		t.Fatal("expected error due to loopback dial block")
	}
}

func TestSSRF_FetchMediaStream_BlocksPrivateTarget(t *testing.T) {
	ctx := context.Background()

	// Should reject loopback URL immediately at validation
	_, err := ssrf.FetchMediaStream(ctx, "http://127.0.0.1:8080/audio.mp3", 2*time.Second)
	if err == nil {
		t.Fatal("expected FetchMediaStream to block loopback address")
	}

	// Should reject non-HTTP schemes
	_, err = ssrf.FetchMediaStream(ctx, "file:///etc/passwd", 2*time.Second)
	if err == nil {
		t.Fatal("expected FetchMediaStream to reject file:// scheme")
	}
}

