package updater

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func TestCheckParsesLatestRelease(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodGet || req.Header.Get("Accept") != "application/vnd.github+json" {
			t.Fatalf("unexpected request: %s %v", req.Method, req.Header)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"tag_name":"v1.2.0"}`)),
			Header:     make(http.Header),
		}, nil
	})}
	version, err := check(context.Background(), "1.1.9", "https://example.test/release", client)
	if err != nil {
		t.Fatal(err)
	}
	if version.Current != "1.1.9" || version.Latest != "1.2.0" || !version.Available {
		t.Fatalf("unexpected version: %+v", version)
	}
}

func TestCheckRejectsInvalidResponse(t *testing.T) {
	for _, response := range []struct {
		name string
		code int
		body string
	}{
		{name: "http error", code: http.StatusServiceUnavailable, body: `{}`},
		{name: "invalid json", code: http.StatusOK, body: `{`},
		{name: "invalid version", code: http.StatusOK, body: `{"tag_name":"latest"}`},
	} {
		t.Run(response.name, func(t *testing.T) {
			client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: response.code,
					Body:       io.NopCloser(strings.NewReader(response.body)),
					Header:     make(http.Header),
				}, nil
			})}
			if _, err := check(context.Background(), "1.0.0", "https://example.test/release", client); err == nil {
				t.Fatal("expected update check error")
			}
		})
	}
}

func TestCheckReportsNetworkFailure(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("connection refused")
	})}
	if _, err := check(context.Background(), "1.0.0", "https://example.test/release", client); err == nil {
		t.Fatal("expected network error")
	}
}

func TestCheckDoesNotOfferSameOrOlderRelease(t *testing.T) {
	for _, latest := range []string{"v1.2.3", "v1.2.2"} {
		client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"tag_name":"` + latest + `"}`)),
				Header:     make(http.Header),
			}, nil
		})}
		version, err := check(context.Background(), "v1.2.3", "https://example.test/release", client)
		if err != nil {
			t.Fatal(err)
		}
		if version.Available {
			t.Fatalf("unexpected update for latest %s", latest)
		}
	}
}

func TestCheckRequiresValidEmbeddedVersion(t *testing.T) {
	if _, err := Check(context.Background(), "dev"); err == nil {
		t.Fatal("expected invalid embedded version error")
	}
}

func TestParseVersionRejectsNonSemver(t *testing.T) {
	for _, version := range []string{"", "v1.2", "1.2.3-beta..1", "01.2.3", "1.x.3"} {
		if _, err := parseVersion(version); err == nil {
			t.Errorf("accepted invalid version %q", version)
		}
	}
}

func TestPrereleaseDoesNotOutrankMatchingStable(t *testing.T) {
	beta, err := parseVersion("v0.5.0-beta.1")
	if err != nil {
		t.Fatal(err)
	}
	stable, err := parseVersion("0.5.0")
	if err != nil {
		t.Fatal(err)
	}
	if compareVersion(stable, beta) <= 0 {
		t.Fatal("matching stable release must outrank beta")
	}
}
