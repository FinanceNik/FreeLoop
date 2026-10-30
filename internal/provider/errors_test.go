package provider

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"testing"
	"time"
)

func TestRetryable(t *testing.T) {
	resp := &http.Response{StatusCode: 429, Header: http.Header{"Retry-After": {"7"}}}
	if ok, after := Retryable(NewHTTPError(resp, "slow down")); !ok || after != 7*time.Second {
		t.Errorf("429 with Retry-After = %v, %v", ok, after)
	}
	for status, want := range map[int]bool{408: true, 500: true, 503: true, 529: true, 501: false, 400: false, 401: false, 404: false} {
		if ok, _ := Retryable(&HTTPError{Status: status}); ok != want {
			t.Errorf("Retryable(%d) = %v", status, ok)
		}
	}
	if ok, _ := Retryable(errors.New("dial tcp: refused")); ok {
		t.Error("a plain error is retryable")
	}
}

// A dropped connection or a response cut off before it began is retried;
// any other plain error is not.
func TestTransientFailures(t *testing.T) {
	for err, want := range map[error]bool{
		fmt.Errorf("read: %w", io.ErrUnexpectedEOF):                  true,
		&url.Error{Op: "Post", URL: "u", Err: timeoutErr{}}:          true,
		errors.New("read tcp 1.2.3.4:443: connection reset by peer"): true,
		errors.New("local-only mode blocks the connection to x"):     false,
		errors.New("invalid model"):                                  false,
	} {
		if got, _ := Retryable(err); got != want {
			t.Errorf("Retryable(%v) = %v", err, got)
		}
	}
}

type timeoutErr struct{}

func (timeoutErr) Error() string   { return "i/o timeout" }
func (timeoutErr) Timeout() bool   { return true }
func (timeoutErr) Temporary() bool { return true }
