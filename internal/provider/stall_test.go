package provider

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestAQuietStreamEnds(t *testing.T) {
	old := StallTimeout
	StallTimeout = 100 * time.Millisecond
	defer func() { StallTimeout = old }()
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("first"))
		w.(http.Flusher).Flush()
		if r.URL.Path == "/slow" {
			for range 4 {
				time.Sleep(50 * time.Millisecond)
				w.Write([]byte("more"))
				w.(http.Flusher).Flush()
			}
			return
		}
		<-release
	}))
	defer srv.Close()
	defer close(release)
	resp, err := KeyClient().Get(srv.URL + "/quiet")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if _, err := io.ReadAll(resp.Body); !errors.Is(err, ErrStalled) {
		t.Errorf("a quiet stream gave %v", err)
	}
	if retry, _ := Retryable(ErrStalled); !retry {
		t.Error("a stalled call is not retried")
	}
	// Slow is not stalled: each byte starts the wait over.
	resp, err = KeyClient().Get(srv.URL + "/slow")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if _, err := io.ReadAll(resp.Body); err != nil {
		t.Errorf("a slow stream gave %v", err)
	}
}
