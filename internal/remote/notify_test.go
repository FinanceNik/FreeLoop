package remote

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/FinanceNik/Freeloop/internal/engine"
)

// pushOf waits for one push to the fake service and reads what it carries.
func pushOf(t *testing.T, f *pushed, read func([]byte) []byte) map[string]any {
	t.Helper()
	select {
	case <-f.got:
	case <-time.After(10 * time.Second):
		t.Fatal("no push arrived")
	}
	var out map[string]any
	if err := json.Unmarshal(read(<-f.body), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func noPush(t *testing.T, f *pushed, why string) {
	t.Helper()
	select {
	case <-f.got:
		t.Errorf("a push went out: %s", why)
	case <-time.After(500 * time.Millisecond):
	}
}

func notifiedSetup(t *testing.T) (*engine.Engine, *Server, *phone, *pushed, func([]byte) []byte) {
	t.Helper()
	e, s, p := started(t)
	f := fakePush(t, s, http.StatusCreated)
	_, ua, auth := subscribed(t, s, p, "https://fcm.googleapis.com/fcm/send/abc")
	return e, s, p, f, func(b []byte) []byte {
		plain, err := decryptPush(b, ua, auth)
		if err != nil {
			t.Fatal(err)
		}
		return plain
	}
}

// 029: an approval in a demo session pushes once to a phone that turned it on and
// holds no stream, and carries the kind and the session and nothing else.
func TestAnApprovalPushesOnce(t *testing.T) {
	e, _, _, f, read := notifiedSetup(t)
	ses, err := e.StartDemo(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	got := pushOf(t, f, read)
	if len(got) != 2 || got["kind"] != "approval" || got["sessionId"] != ses.ID {
		t.Errorf("payload: %v", got)
	}
	noPush(t, f, "a second one for the same request")
}

// 029: a phone with a stream open is looking at the sessions, and is not pushed to.
func TestNoPushToAPhoneWithAStreamOpen(t *testing.T) {
	e, _, p, f, _ := notifiedSetup(t)
	req, _ := http.NewRequest("GET", p.base+"/api/stream", nil)
	p.authorize(req)
	resp, err := p.client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("stream: %v", err)
	}
	defer resp.Body.Close()
	bufio.NewReader(resp.Body).ReadString('\n') // it is registered once it answers
	if _, err := e.StartDemo(context.Background()); err != nil {
		t.Fatal(err)
	}
	noPush(t, f, "to a phone whose stream is open")
}

// 029: notifications turned off push nothing.
func TestNoPushWhenNotificationsAreOff(t *testing.T) {
	e, _, _, f, _ := notifiedSetup(t)
	cfg := e.Config()
	cfg.Notifications.Enabled = false
	if _, err := e.SaveSettings(cfg); err != nil {
		t.Fatal(err)
	}
	if _, err := e.StartDemo(context.Background()); err != nil {
		t.Fatal(err)
	}
	noPush(t, f, "with notifications off")
}

// 029: a muted project pushes nothing: the first request pushes, then the project is
// muted and the request answered, and the next does not.
func TestNoPushForAMutedProject(t *testing.T) {
	e, _, _, f, read := notifiedSetup(t)
	ses, err := e.StartDemo(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	pushOf(t, f, read)
	if err := e.SetProjectMuted(context.Background(), ses.ProjectID, true); err != nil {
		t.Fatal(err)
	}
	for _, r := range e.PendingPermissions() {
		e.Decide(engine.DecideRequest{RequestID: r.ID, OptionID: "allow_once"})
	}
	noPush(t, f, "for a muted project")
}

// 029: a notice's kind becomes the kind the worker has words for, and nothing is
// pushed in local-only mode.
func TestNoticeKindsAndLocalOnly(t *testing.T) {
	for in, want := range map[string]string{"approval": "approval", "finished": "done", "error": "failed", "budget": "budget", "x": "other"} {
		if got := pushKind(in); got != want {
			t.Errorf("%s: %s, want %s", in, got, want)
		}
	}
	e, s, _, f, _ := notifiedSetup(t)
	cfg := e.Config()
	cfg.Network.LocalOnly = true
	if _, err := e.SaveSettings(cfg); err != nil {
		t.Fatal(err)
	}
	s.Notify(engine.Notice{Kind: "finished", SessionID: "ses_1"})
	noPush(t, f, "in local-only mode")
}
