package provider

import (
	"errors"
	"net/http"
	"strconv"
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
		return false, 0
	}
	switch {
	case h.Status == http.StatusRequestTimeout, h.Status == http.StatusTooManyRequests:
		return true, h.RetryAfter
	case h.Status >= 500 && h.Status != http.StatusNotImplemented:
		return true, h.RetryAfter
	}
	return false, 0
}
