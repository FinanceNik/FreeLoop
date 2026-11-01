package netguard

import (
	"net/url"
	"strings"
)

// RuntimeCallAllowed reports whether a request to the app's runtime
// endpoint, which reaches every engine method, comes from the app's own
// page and not from something else its browser engine loaded, such as a
// page in the preview frame (GUI-020). Wails makes such a check only in its
// server build; its desktop build takes every request.
//
// Tried against WebKit in the packaged app, a framed page's requests look
// like this, and the app's own always carry the client header and no origin:
//
//   - a no-cors fetch reaches the app's URL scheme with its origin stripped,
//     but a page cannot set a custom header on one: the client header is
//     required;
//   - a fetch with custom headers gets no preflight there and arrives with
//     the header, but keeps its origin: a foreign origin is refused;
//   - a form post keeps its origin and has no header.
//
// In the desktop app no http origin is taken at all. A page chooses the host
// of a request it sends to the app's URL scheme, so comparing its origin
// with that host proves nothing: fetch("wails://localhost:41000/...") from
// http://localhost:41000 would match. Only the server build, whose requests
// arrive over a real socket and whose page is served from the address it
// listens on, compares them (served).
//
// Only POST is taken, because a GET can be sent by a mere image tag.
func RuntimeCallAllowed(method, origin, fetchSite, host, clientID string, served bool) bool {
	if method != "POST" || clientID == "" {
		return false
	}
	if origin == "" {
		return fetchSite != "cross-site" && fetchSite != "same-site"
	}
	u, err := url.Parse(origin)
	if err != nil || origin == "null" {
		return false
	}
	if served {
		return strings.EqualFold(u.Host, host)
	}
	// The app's page is served from its own URL scheme in the desktop app.
	return u.Scheme == "wails"
}
