package provider

import (
	"errors"
	"net/http"
	"strings"
)

// StayOnHost is an http.Client's CheckRedirect for a request carrying a key. Go
// drops only Authorization and Cookie on a redirect to another host, so an
// X-Api-Key, or any header from providers.yaml, would go along, and a 307 sends
// the prompt too. A redirect to plain http would send them unencrypted. Neither
// is followed, so a gateway sending an expired session to its login page, say,
// costs an error and not the key.
func StayOnHost(next *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return errors.New("the endpoint redirected too many times")
	}
	first := via[0].URL
	if !strings.EqualFold(next.URL.Host, first.Host) {
		return errors.New("the endpoint redirects to another address, where the API key is not sent: " + next.URL.Host)
	}
	if first.Scheme == "https" && next.URL.Scheme != "https" {
		return errors.New("the endpoint redirects from https to plain http, where the API key is not sent")
	}
	return nil
}

// KeyClient is the client for requests carrying a key, through
// http.DefaultTransport, so local-only mode covers it (NFR-OFF-003).
func KeyClient() *http.Client {
	return &http.Client{CheckRedirect: StayOnHost}
}
