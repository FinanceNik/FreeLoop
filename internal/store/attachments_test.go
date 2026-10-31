package store

import (
	"bytes"
	"context"
	"testing"

	"github.com/FinanceNik/Freeloop/internal/session"
)

func TestAttachments(t *testing.T) {
	s := openTest(t, t.TempDir(), nil)
	ctx := context.Background()
	a, _ := s.CreateSession(ctx, session.Session{Title: "a", ProjectRoot: t.TempDir()})
	b, _ := s.CreateSession(ctx, session.Session{Title: "b", ProjectRoot: t.TempDir()})
	png := []byte("\\x89PNG\\r\\n\\x1a\\n fake")
	name, err := s.SaveAttachment(a.ID, "image/png", png)
	if err != nil {
		t.Fatal(err)
	}
	if again, _ := s.SaveAttachment(a.ID, "image/png", png); again != name || AttachmentType(name) != "image/png" {
		t.Errorf("second save = %s, type %s", again, AttachmentType(name))
	}
	if got, err := s.Attachment(a.ID, name); err != nil || !bytes.Equal(got, png) {
		t.Errorf("read = %q, %v", got, err)
	}
	for _, bad := range []string{"../../etc/passwd", name + "/x", "x.png"} {
		if _, err := s.Attachment(a.ID, bad); err == nil {
			t.Errorf("read %q", bad)
		}
	}
	if _, err := s.SaveAttachment(a.ID, "image/svg+xml", []byte("<svg/>")); err == nil {
		t.Error("an SVG was stored")
	}
	if err := s.CopyAttachments(a.ID, b.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Attachment(b.ID, name); err != nil {
		t.Errorf("the fork lacks the attachment: %v", err)
	}
}
