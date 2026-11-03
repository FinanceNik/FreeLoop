package provider

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"runtime/debug"
	"strconv"
	"strings"
	"time"
)

// HTTPError is a provider's HTTP failure, with what a retry needs (PRV-010).
type HTTPError struct {
	Status int
	// RetryAfter is the server's Retry-After, when it sent one.
	RetryAfter time.Duration
	Message    string
}

func (e *HTTPError) Error() string { return e.Message }

// NewHTTPError reads the status and Retry-After of resp.
func NewHTTPError(resp *http.Response, message string) *HTTPError {
	e := &HTTPError{Status: resp.StatusCode, Message: message}
	if s := resp.Header.Get("Retry-After"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n >= 0 {
			e.RetryAfter = time.Duration(n) * time.Second
		} else if t, err := http.ParseTime(s); err == nil {
			e.RetryAfter = max(0, time.Until(t))
		}
	}
	return e
}

// Retryable reports whether a request that failed with err may succeed
// when sent again: a timeout, a rate limit or a server error; and how long
// the server asked to wait, if it did.
func Retryable(err error) (bool, time.Duration) {
	var h *HTTPError
	if !errors.As(err, &h) {
		return transient(err), 0
	}
	switch {
	case h.Status == http.StatusRequestTimeout, h.Status == http.StatusTooManyRequests:
		return true, h.RetryAfter
	case h.Status >= 500 && h.Status != http.StatusNotImplemented:
		return true, h.RetryAfter
	}
	return false, 0
}

// transient reports a failure of the connection rather than of the
// request: a timeout, a connection the server or network dropped, or a
// response cut off before it began. (Local-only refusals and cancellations
// are not: the harness checks the context first.)
func transient(err error) bool {
	if errors.Is(err, ErrStalled) || errors.Is(err, ErrStreamEnded) {
		return true
	}
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return true
	}
	if errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "connection reset by peer") || strings.Contains(msg, "broken pipe")
}

// RecoverStream, deferred in a stream goroutine after close, turns a panic
// into an error event: one malformed response from an endpoint, or a bug
// it hits in an SDK, fails the turn instead of the app.
func RecoverStream(ch chan<- Event) {
	r := recover()
	if r == nil {
		return
	}
	log.Printf("provider: stream panic: %v\n%s", r, debug.Stack())
	select {
	case ch <- Event{Type: EventError, Err: fmt.Errorf("the provider sent a response that could not be read (%v)", r)}:
	default:
	}
}
