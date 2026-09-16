package httpx

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Harrison-Blair/qmeter/internal/provider"
)

type usageBody struct {
	Plan    string  `json:"plan"`
	Percent float64 `json:"percent"`
}

func TestGet_DecodesJSON(t *testing.T) {
	t.Parallel()

	var gotMethod, gotPath, gotAuth, gotBeta string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotBeta = r.Header.Get("anthropic-beta")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"plan":"max","percent":42.5}`))
	}))
	defer srv.Close()

	var got usageBody
	err := Get(context.Background(), Options{
		URL:  srv.URL + "/api/oauth/usage",
		Tool: "claude",
		Headers: map[string]string{
			"Authorization":  "Bearer tok",
			"anthropic-beta": "oauth-2025-04-20",
		},
	}, &got)
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if gotMethod != http.MethodGet {
		t.Errorf("method = %q, want %q", gotMethod, http.MethodGet)
	}
	if gotPath != "/api/oauth/usage" {
		t.Errorf("path = %q, want %q", gotPath, "/api/oauth/usage")
	}
	if gotAuth != "Bearer tok" {
		t.Errorf("Authorization = %q, want %q", gotAuth, "Bearer tok")
	}
	if gotBeta != "oauth-2025-04-20" {
		t.Errorf("anthropic-beta = %q, want %q", gotBeta, "oauth-2025-04-20")
	}
	if want := (usageBody{Plan: "max", Percent: 42.5}); got != want {
		t.Errorf("decoded = %+v, want %+v", got, want)
	}
}

func TestGet_NilTargetDiscardsBody(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"plan":"max"}`))
	}))
	defer srv.Close()

	if err := Get(context.Background(), Options{URL: srv.URL, Tool: "claude"}, nil); err != nil {
		t.Fatalf("Get with nil target returned error: %v", err)
	}
}

// statusServer replies with the given status, headers and body for every
// request.
func statusServer(t *testing.T, status int, headers map[string]string, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for k, v := range headers {
			w.Header().Set(k, v)
		}
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestGet_401MapsToTokenExpired(t *testing.T) {
	t.Parallel()

	srv := statusServer(t, http.StatusUnauthorized, nil, `{"error":"invalid token"}`)

	var got usageBody
	err := Get(context.Background(), Options{URL: srv.URL, Tool: "claude"}, &got)
	if !errors.Is(err, provider.ErrTokenExpired{}) {
		t.Fatalf("err = %v, want provider.ErrTokenExpired", err)
	}
	var te provider.ErrTokenExpired
	if !errors.As(err, &te) {
		t.Fatalf("errors.As(%v, *provider.ErrTokenExpired) = false", err)
	}
	if te.Tool != "claude" {
		t.Errorf("Tool = %q, want %q", te.Tool, "claude")
	}
}

func TestGet_403MapsToTokenExpired(t *testing.T) {
	t.Parallel()

	srv := statusServer(t, http.StatusForbidden, nil, "forbidden")

	var te provider.ErrTokenExpired
	err := Get(context.Background(), Options{URL: srv.URL, Tool: "cursor-agent"}, &usageBody{})
	if !errors.As(err, &te) {
		t.Fatalf("err = %v, want provider.ErrTokenExpired", err)
	}
	if te.Tool != "cursor-agent" {
		t.Errorf("Tool = %q, want %q", te.Tool, "cursor-agent")
	}
}

func TestGet_429MapsToErrRateLimitedWithRetryAfter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		retryAfter string
		want       time.Duration
	}{
		{name: "integer seconds", retryAfter: "120", want: 120 * time.Second},
		{name: "zero seconds", retryAfter: "0", want: 0},
		{name: "surrounding space", retryAfter: " 90 ", want: 90 * time.Second},
		{name: "header absent", retryAfter: "", want: 0},
		{name: "http date is unparseable", retryAfter: "Wed, 21 Oct 2026 07:28:00 GMT", want: 0},
		{name: "garbage", retryAfter: "soon", want: 0},
		{name: "negative", retryAfter: "-5", want: 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			headers := map[string]string{}
			if tc.retryAfter != "" {
				headers["Retry-After"] = tc.retryAfter
			}
			srv := statusServer(t, http.StatusTooManyRequests, headers, "slow down")

			err := Get(context.Background(), Options{URL: srv.URL, Tool: "claude"}, &usageBody{})
			if !errors.Is(err, provider.ErrRateLimited{}) {
				t.Fatalf("err = %v, want provider.ErrRateLimited", err)
			}
			var rl provider.ErrRateLimited
			if !errors.As(err, &rl) {
				t.Fatalf("errors.As(%v, *provider.ErrRateLimited) = false", err)
			}
			if rl.RetryAfter != tc.want {
				t.Errorf("RetryAfter = %v, want %v", rl.RetryAfter, tc.want)
			}
		})
	}
}

func TestGet_5xxReturnsStatusAndBodyPrefix(t *testing.T) {
	t.Parallel()

	srv := statusServer(t, http.StatusInternalServerError, nil, "upstream exploded")

	err := Get(context.Background(), Options{URL: srv.URL, Tool: "claude"}, &usageBody{})
	if err == nil {
		t.Fatal("Get returned nil error for 500")
	}
	if errors.Is(err, provider.ErrTokenExpired{}) || errors.Is(err, provider.ErrRateLimited{}) {
		t.Fatalf("err = %v, want a plain error, not a typed provider error", err)
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("err = %q, want it to carry the status code 500", err)
	}
	if !strings.Contains(err.Error(), "upstream exploded") {
		t.Errorf("err = %q, want it to carry the body prefix", err)
	}
}

func TestGet_404ReturnsStatusAndBodyPrefix(t *testing.T) {
	t.Parallel()

	srv := statusServer(t, http.StatusNotFound, nil, "no such route")

	err := Get(context.Background(), Options{URL: srv.URL, Tool: "claude"}, &usageBody{})
	if err == nil {
		t.Fatal("Get returned nil error for 404")
	}
	if !strings.Contains(err.Error(), "404") || !strings.Contains(err.Error(), "no such route") {
		t.Errorf("err = %q, want status and body prefix", err)
	}
}

func TestGet_NonJSONBodyReturnsDecodeError(t *testing.T) {
	t.Parallel()

	srv := statusServer(t, http.StatusOK, map[string]string{"Content-Type": "text/html"}, "<html>not json</html>")

	var got usageBody
	err := Get(context.Background(), Options{URL: srv.URL, Tool: "claude"}, &got)
	if err == nil {
		t.Fatal("Get returned nil error for a non-JSON body")
	}
	if errors.Is(err, provider.ErrTokenExpired{}) || errors.Is(err, provider.ErrRateLimited{}) {
		t.Fatalf("err = %v, want a plain decode error", err)
	}
}

func TestGet_CapsBodyRead(t *testing.T) {
	t.Parallel()

	// A complete, well-formed JSON document larger than the cap: it would
	// decode cleanly if httpx read the whole thing, so a successful decode
	// here proves the cap is missing.
	huge := `{"plan":"` + strings.Repeat("x", int(maxBodyBytes)+4096) + `"}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, huge)
	}))
	defer srv.Close()

	var got usageBody
	err := Get(context.Background(), Options{URL: srv.URL, Tool: "claude"}, &got)
	if err == nil {
		t.Fatalf("Get decoded a %d-byte body; want the read capped at %d bytes", len(huge), maxBodyBytes)
	}
	if len(got.Plan) > int(maxBodyBytes) {
		t.Errorf("decoded %d bytes into the target, want at most %d", len(got.Plan), maxBodyBytes)
	}
}

func TestGet_CapsErrorBodyPrefix(t *testing.T) {
	t.Parallel()

	srv := statusServer(t, http.StatusBadGateway, nil, strings.Repeat("z", 2<<20))

	err := Get(context.Background(), Options{URL: srv.URL, Tool: "claude"}, &usageBody{})
	if err == nil {
		t.Fatal("Get returned nil error for 502")
	}
	if len(err.Error()) > 1024 {
		t.Errorf("error message is %d bytes; want a bounded body prefix", len(err.Error()))
	}
}

func TestGet_RespectsCallerContextDeadline(t *testing.T) {
	t.Parallel()

	release := make(chan struct{})
	defer close(release)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	err := Get(ctx, Options{URL: srv.URL, Tool: "claude"}, &usageBody{})
	elapsed := time.Since(start)

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want it to wrap context.DeadlineExceeded", err)
	}
	if elapsed > 5*time.Second {
		t.Errorf("Get took %v; want it to return at the caller's deadline", elapsed)
	}
}

func TestGet_SetsNoTimeoutOfItsOwn(t *testing.T) {
	t.Parallel()

	if DefaultClient.Timeout != 0 {
		t.Errorf("DefaultClient.Timeout = %v, want 0 — the deadline comes from the caller's context", DefaultClient.Timeout)
	}

	srv := statusServer(t, http.StatusOK, nil, `{"plan":"max"}`)
	client := &http.Client{}
	if err := Get(context.Background(), Options{URL: srv.URL, Tool: "claude", Client: client}, &usageBody{}); err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if client.Timeout != 0 {
		t.Errorf("caller's client.Timeout = %v after Get, want it left at 0", client.Timeout)
	}
}

func TestPost_SendsBodyAndHeaders(t *testing.T) {
	t.Parallel()

	var gotMethod, gotCT, gotProto, gotAuth string
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotCT = r.Header.Get("Content-Type")
		gotProto = r.Header.Get("Connect-Protocol-Version")
		gotAuth = r.Header.Get("Authorization")
		gotBody, _ = io.ReadAll(r.Body)
		_, _ = io.WriteString(w, `{"plan":"pro","percent":7.5}`)
	}))
	defer srv.Close()

	var got usageBody
	err := Post(context.Background(), Options{
		URL:  srv.URL + "/aiserver.v1.DashboardService/GetCurrentPeriodUsage",
		Tool: "cursor-agent",
		Headers: map[string]string{
			"Authorization":            "Bearer tok",
			"Content-Type":             "application/json",
			"Connect-Protocol-Version": "1",
		},
	}, []byte(`{}`), &got)
	if err != nil {
		t.Fatalf("Post returned error: %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want %q", gotMethod, http.MethodPost)
	}
	if string(gotBody) != `{}` {
		t.Errorf("body = %q, want %q", gotBody, `{}`)
	}
	if gotAuth != "Bearer tok" {
		t.Errorf("Authorization = %q, want %q", gotAuth, "Bearer tok")
	}
	if gotCT != "application/json" {
		t.Errorf("Content-Type = %q, want %q", gotCT, "application/json")
	}
	if gotProto != "1" {
		t.Errorf("Connect-Protocol-Version = %q, want %q", gotProto, "1")
	}
	if want := (usageBody{Plan: "pro", Percent: 7.5}); got != want {
		t.Errorf("decoded = %+v, want %+v", got, want)
	}
}

func TestPost_401MapsToTokenExpired(t *testing.T) {
	t.Parallel()

	srv := statusServer(t, http.StatusUnauthorized, nil, "nope")

	var te provider.ErrTokenExpired
	err := Post(context.Background(), Options{URL: srv.URL, Tool: "codex"}, []byte(`{}`), &usageBody{})
	if !errors.As(err, &te) {
		t.Fatalf("err = %v, want provider.ErrTokenExpired", err)
	}
	if te.Tool != "codex" {
		t.Errorf("Tool = %q, want %q", te.Tool, "codex")
	}
}

func TestGet_2xxOtherThan200IsSuccess(t *testing.T) {
	t.Parallel()

	for _, status := range []int{http.StatusCreated, http.StatusAccepted, http.StatusNoContent} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			t.Parallel()

			srv := statusServer(t, status, nil, "")

			if err := Get(context.Background(), Options{URL: srv.URL, Tool: "claude"}, nil); err != nil {
				t.Fatalf("Get on %d returned error %v, want nil — every 2xx is a success", status, err)
			}
		})
	}
}

func TestGet_EmptyBodyWithTargetReportsEmptyResponse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		status int
	}{
		{name: "204 no content", status: http.StatusNoContent},
		{name: "200 with no bytes", status: http.StatusOK},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			srv := statusServer(t, tc.status, nil, "")

			var got usageBody
			err := Get(context.Background(), Options{URL: srv.URL, Tool: "claude"}, &got)
			if err == nil {
				t.Fatal("Get returned nil error for an empty body with a non-nil target")
			}
			want := fmt.Sprintf("httpx: GET %s: empty response body (status %d)", srv.URL, tc.status)
			if err.Error() != want {
				t.Errorf("err = %q, want %q", err.Error(), want)
			}
		})
	}
}
