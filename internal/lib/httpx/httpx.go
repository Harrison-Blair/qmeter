// Package httpx holds the shared, context-aware HTTP fetch helper every
// provider package uses: Get and Post send caller-supplied headers, decode a
// JSON response body into a caller-supplied target, and map non-2xx
// responses to the typed errors in internal/provider.
//
// httpx sets no timeout of its own — the deadline comes entirely from the
// caller's context (internal/usage owns DefaultProviderTimeout). Response
// bodies are read through a cap so a hostile server cannot exhaust memory.
package httpx

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Harrison-Blair/qmeter/internal/provider"
)

// maxBodyBytes caps how much of a response body httpx will read, so a
// hostile or broken server cannot exhaust memory.
const maxBodyBytes int64 = 1 << 20

// errBodyPrefixBytes bounds how much of a non-2xx body is quoted back in an
// error message.
const errBodyPrefixBytes = 512

// DefaultClient is the http.Client used when Options.Client is nil.
var DefaultClient = &http.Client{}

// Options describes a single request.
type Options struct {
	// URL is the full request URL.
	URL string

	// Tool is the vendor CLI name used in the "open <tool>" hint of a
	// provider.ErrTokenExpired built from a 401 or 403 (one of: claude,
	// codex, cursor-agent, opencode).
	Tool string

	// Headers are set verbatim on the request, one value per name.
	Headers map[string]string

	// Client is the http.Client to use. When nil, DefaultClient is used.
	// httpx never sets a Timeout on either one.
	Client *http.Client
}

// Get issues a GET and decodes the JSON response body into out. A nil out
// discards the body.
//
// A non-2xx response maps to a typed error: 401 and 403 to
// provider.ErrTokenExpired{Tool: opts.Tool}, 429 to
// provider.ErrRateLimited with the parsed retry-after, and every other
// status to a plain error carrying the status code and a bounded prefix of
// the body. Errors are returned by value, never by pointer, so
// errors.Is/errors.As with a value target match.
func Get(ctx context.Context, opts Options, out any) error {
	return do(ctx, http.MethodGet, opts, nil, out)
}

// Post issues a POST with body as the request body and decodes the JSON
// response body into out, mapping errors exactly as Get does. The caller
// supplies any Content-Type it needs via Options.Headers.
func Post(ctx context.Context, opts Options, body []byte, out any) error {
	return do(ctx, http.MethodPost, opts, body, out)
}

// do performs one request. It sets no timeout of its own: the deadline comes
// entirely from ctx.
func do(ctx context.Context, method string, opts Options, body []byte, out any) error {
	var reqBody io.Reader
	if body != nil {
		reqBody = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, opts.URL, reqBody)
	if err != nil {
		return fmt.Errorf("httpx: new %s request: %w", method, err)
	}
	for k, v := range opts.Headers {
		req.Header.Set(k, v)
	}
	client := opts.Client
	if client == nil {
		client = DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("httpx: %s %s: %w", method, opts.URL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return statusError(method, opts, resp)
	}
	// Cap every read: a hostile server must not be able to stream an
	// unbounded body into memory.
	limited := io.LimitReader(resp.Body, maxBodyBytes)
	if out == nil {
		_, _ = io.Copy(io.Discard, limited)
		return nil
	}
	if err := json.NewDecoder(limited).Decode(out); err != nil {
		return fmt.Errorf("httpx: decode %s response: %w", opts.URL, err)
	}
	return nil
}

// statusError maps a non-2xx response to the error the providers expect:
// 401/403 to provider.ErrTokenExpired, 429 to provider.ErrRateLimited with
// the parsed retry-after, anything else to a plain error carrying the
// status code and a bounded prefix of the body.
func statusError(method string, opts Options, resp *http.Response) error {
	switch resp.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return provider.ErrTokenExpired{Tool: opts.Tool}
	case http.StatusTooManyRequests:
		return provider.ErrRateLimited{RetryAfter: parseRetryAfter(resp.Header.Get("Retry-After"))}
	default:
		return fmt.Errorf("httpx: %s %s: unexpected status %d: %s",
			method, opts.URL, resp.StatusCode, bodyPrefix(resp.Body))
	}
}

// parseRetryAfter reads a retry-after header value as whole seconds. The
// HTTP-date form is deliberately treated as unparseable and yields zero, as
// does a missing, negative or malformed value.
func parseRetryAfter(v string) time.Duration {
	secs, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil || secs < 0 {
		return 0
	}
	return time.Duration(secs) * time.Second
}

// bodyPrefix reads at most errBodyPrefixBytes of body for use in an error
// message.
func bodyPrefix(body io.Reader) string {
	b, _ := io.ReadAll(io.LimitReader(body, errBodyPrefixBytes))
	return strings.TrimSpace(string(b))
}
