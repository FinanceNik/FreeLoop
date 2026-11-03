package engine

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/harness"
	"github.com/FinanceNik/Freeloop/internal/netguard"
	"github.com/FinanceNik/Freeloop/internal/secrets"
	"github.com/FinanceNik/Freeloop/internal/web"
)

// The engine's side of the built-in harness's web tools (HRN-017). A fetch
// reaches public addresses only, so a page or a model cannot point the app
// at services on this Mac or its network. The only search endpoint is one
// the user configured (OQ-025), which may be on this Mac.

const (
	maxPageBytes  = 5 << 20
	maxPageText   = 50 << 10
	maxSearchBody = 2 << 20
	maxHits       = 10
)

// webSearchKey is where the search endpoint's key is kept.
var webSearchKey = secrets.Ref{Service: "freeloop", Account: "web/search"}

type sessionWeb struct {
	e *Engine
	// client replaces the public-only client in tests, whose pages are on
	// this Mac.
	client *http.Client
}

func (w sessionWeb) CanSearch() bool { return w.e.searchApproved(w.e.Config().Web.SearchURL) }

// searchApproved reports a search address that queries may go to.
func (e *Engine) searchApproved(address string) bool {
	return address != "" && e.commandApproved(config.AddressFingerprint("web.searchURL", address))
}

func (w sessionWeb) Fetch(ctx context.Context, raw string) (harness.WebPage, error) {
	if netguard.LocalOnly() {
		return harness.WebPage{}, errors.New("the app is set to connect to nothing but this Mac")
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return harness.WebPage{}, fmt.Errorf("%q is not an http or https address", raw)
	}
	if u.User != nil {
		return harness.WebPage{}, errors.New("an address with a user name or password is not fetched")
	}
	client := w.client
	if client == nil {
		if hostIsPrivate(u.Hostname()) {
			return harness.WebPage{}, fmt.Errorf("refused %s: a fetch may not reach a private address", u.Host)
		}
		client = publicClient()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return harness.WebPage{}, err
	}
	req.Header.Set("User-Agent", "Freeloop")
	req.Header.Set("Accept", "text/html,text/plain,application/json;q=0.9,*/*;q=0.1")
	res, err := client.Do(req)
	if err != nil {
		return harness.WebPage{}, err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return harness.WebPage{}, fmt.Errorf("%s answered %s", u.Host, res.Status)
	}
	media, _, _ := mime.ParseMediaType(res.Header.Get("Content-Type"))
	if !readable(media) {
		return harness.WebPage{}, fmt.Errorf("%s is %s, not a page that can be read as text", u.Host, media)
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, maxPageBytes+1))
	if err != nil {
		return harness.WebPage{}, err
	}
	page := harness.WebPage{URL: res.Request.URL.String(), Truncated: len(b) > maxPageBytes}
	text := strings.ToValidUTF8(string(b), "")
	if media == "text/html" || media == "application/xhtml+xml" || media == "" {
		page.Title, text = web.Text(text)
	}
	if len(text) > maxPageText {
		text, page.Truncated = cutBytes(text, maxPageText), true
	}
	page.Text = text
	return page, nil
}

func readable(media string) bool {
	switch {
	case media == "", strings.HasPrefix(media, "text/"), media == "application/json", media == "application/xml", media == "application/xhtml+xml", strings.HasSuffix(media, "+json"), strings.HasSuffix(media, "+xml"):
		return true
	}
	return false
}

func (w sessionWeb) Search(ctx context.Context, query string) ([]web.Result, error) {
	cfg := w.e.Config().Web
	if cfg.SearchURL == "" {
		return nil, errors.New("no search endpoint is configured")
	}
	raw := strings.ReplaceAll(cfg.SearchURL, "{query}", url.QueryEscape(query))
	inURL := strings.Contains(raw, "{key}")
	key := ""
	if cfg.SearchHeader != "" || inURL {
		var err error
		if key, err = w.e.searchKey(cfg.SearchURL); err != nil {
			return nil, err
		}
		raw = strings.ReplaceAll(raw, "{key}", url.QueryEscape(key))
	}
	if !w.e.searchApproved(cfg.SearchURL) {
		return nil, errors.New("the search endpoint has not been approved; approve it in Settings")
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("the search endpoint is not an http or https address")
	}
	// A key typed into the address by hand, in a file: not kept there on purpose,
	// and not to reach a transcript either.
	for _, v := range credentialsIn(u) {
		w.e.remember(v)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if cfg.SearchHeader != "" {
		req.Header.Set(cfg.SearchHeader, key)
	}
	// Through http.DefaultTransport, so the local-only switch covers it. The
	// key is for the endpoint the user set: a redirect to another host would
	// carry it there.
	client := &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(next *http.Request, via []*http.Request) error {
		if next.URL.Host != req.URL.Host || (req.URL.Scheme == "https" && next.URL.Scheme != "https") || len(via) >= 5 {
			return errors.New("the search endpoint redirects elsewhere")
		}
		return nil
	}}
	res, err := client.Do(req)
	if err != nil {
		// Go's error quotes the whole address, a key in it included, and it goes
		// to the model and the transcript.
		var uerr *url.Error
		if errors.As(err, &uerr) {
			return nil, fmt.Errorf("the search endpoint at %s: %w", u.Host, uerr.Err)
		}
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, fmt.Errorf("the search endpoint answered %s", res.Status)
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, maxSearchBody))
	if err != nil {
		return nil, err
	}
	hits, err := web.ParseResults(b)
	if err != nil {
		return nil, err
	}
	if len(hits) > maxHits {
		hits = hits[:maxHits]
	}
	return hits, nil
}

// SetWebSearchKey keeps the search endpoint's key in the Keychain; empty
// removes it.
func (e *Engine) SetWebSearchKey(key string) error {
	if key = strings.TrimSpace(key); key == "" {
		e.secrets.Delete(webSearchKey)
		return nil
	}
	e.remember(key)
	address := e.Config().Web.SearchURL
	if err := e.secrets.Set(webSearchKey, bindSecret(originOf(address), key)); err != nil {
		return err
	}
	// Entering the key is the user saying this address is where it goes.
	if address == "" {
		return nil
	}
	mark, err := e.approvalMark(config.AddressFingerprint("web.searchURL", address))
	if err != nil {
		return err
	}
	return e.addApprovals([]string{mark})
}
