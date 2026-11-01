package webpair

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func get(t *testing.T, url, host string) (int, string, http.Header) {
	t.Helper()
	req, _ := http.NewRequest("GET", url, nil)
	if host != "" {
		req.Host = host
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body), resp.Header
}

func TestServesQROnlyOnTokenPathAndLocalHost(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	payload := "2@synthetic-fixture,A,B,C"
	s, err := Start(ctx, func() string { return payload }, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Stop()
	if !strings.HasPrefix(s.URL, "http://127.0.0.1:") {
		t.Fatal("must bind loopback", s.URL)
	}
	status, body, header := get(t, s.URL, "")
	if status != 200 || !strings.Contains(body, "<svg") || strings.Contains(body, "synthetic-fixture") {
		t.Fatal("QR page wrong or leaked payload text")
	}
	if header.Get("Cache-Control") != "no-store" || !strings.Contains(header.Get("Content-Security-Policy"), "default-src 'none'") {
		t.Fatal("missing security headers")
	}
	base := s.URL[:strings.LastIndex(strings.TrimSuffix(s.URL, "/"), "/")+1]
	if status, _, _ := get(t, base, ""); status != 404 {
		t.Fatal("served without token")
	}
	if status, _, _ := get(t, s.URL, "evil.example"); status != 404 {
		t.Fatal("accepted foreign Host header")
	}
	if status, _, _ := get(t, s.URL, "localhost:9999"); status != 200 {
		t.Fatal("rejected forwarded localhost port")
	}
	if _, body, _ := get(t, s.URL, ""); strings.Contains(body, "Waiting") {
		t.Fatal("unexpected waiting page")
	}
}

func TestStopsOnContextAndTTL(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	s, err := Start(ctx, func() string { return "" }, 100*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(400 * time.Millisecond)
	if _, err := http.Get(s.URL); err == nil {
		t.Fatal("server still running after TTL")
	}
	cancel()
}
