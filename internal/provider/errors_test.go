package provider

import (
	"errors"
	"net/http"
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
