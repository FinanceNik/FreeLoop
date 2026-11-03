package provider

import (
	"context"
	"errors"
	"io"
	"net/http"
	"time"
)

// ErrStalled is a request that went quiet: no byte arrived for StallTimeout.
var ErrStalled = errors.New("the provider stopped responding")

// ErrStreamEnded is a stream that closed cleanly before the provider said the
// reply was finished, so what arrived may be only part of it.
var ErrStreamEnded = errors.New("the provider's stream ended before the reply finished")

// StallTimeout is how long a request may go without a byte. Hosted providers send
// keep-alives far more often; a local model loading from disk is the slow case,
// which is why this is minutes. Without it one silent stream would hold its slot
// in the provider's concurrency cap forever, and with enough of them every session
// on that provider would wait. Tests shorten it.
var StallTimeout = 5 * time.Minute

// stallTransport ends a request that goes quiet, whatever the provider's SDK does.
type stallTransport struct{}

func (stallTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	idle := StallTimeout
	ctx, cancel := context.WithCancelCause(req.Context())
	timer := time.AfterFunc(idle, func() { cancel(ErrStalled) })
	// Through http.DefaultTransport, so local-only mode covers it (NFR-OFF-003).
	resp, err := http.DefaultTransport.RoundTrip(req.WithContext(ctx))
	if err != nil {
		timer.Stop()
		stalled := errors.Is(context.Cause(ctx), ErrStalled)
		cancel(nil)
		if stalled {
			return nil, ErrStalled
		}
		return nil, err
	}
	timer.Reset(idle)
	resp.Body = &stallBody{ReadCloser: resp.Body, ctx: ctx, cancel: cancel, timer: timer, idle: idle}
	return resp, nil
}

type stallBody struct {
	io.ReadCloser
	ctx    context.Context
	cancel context.CancelCauseFunc
	timer  *time.Timer
	idle   time.Duration
}

func (b *stallBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if n > 0 {
		b.timer.Reset(b.idle)
	}
	if err != nil && err != io.EOF && errors.Is(context.Cause(b.ctx), ErrStalled) {
		return n, ErrStalled
	}
	return n, err
}

func (b *stallBody) Close() error {
	b.timer.Stop()
	err := b.ReadCloser.Close()
	b.cancel(nil)
	return err
}
