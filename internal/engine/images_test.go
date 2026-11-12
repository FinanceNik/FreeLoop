package engine

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/provider"
	"github.com/FinanceNik/Freeloop/internal/provider/scripted"
)

// A 1×1 PNG.
var tinyPNG, _ = base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==")

// SES-015: a picture is stored once, named in the transcript, sent to the
// model with its bytes, and loaded from the store again after a restart.
func TestImagesInPrompts(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	e := openTest(t, dir)
	ch, unsub := e.Subscribe()
	p := scripted.New("sp", scripted.Step{Text: "a red dot"}, scripted.Step{Text: "still red"})
	e.mu.Lock()
	e.providers.built["sp"] = &configuredProvider{cfg: config.Provider{ID: "sp", Name: "sp", Type: "scripted"}, p: p}
	e.mu.Unlock()
	ses, err := e.CreateSession(ctx, CreateSessionRequest{ProjectRoot: t.TempDir(), ProviderID: "sp", Model: "m", Mode: "ask"})
	if err != nil {
		t.Fatal(err)
	}
	png := ImageInput{MediaType: "image/png", Data: base64.StdEncoding.EncodeToString(tinyPNG)}
	if _, err := e.PromptWith(ctx, ses.ID, PromptRequest{Text: "what is this?", Images: []ImageInput{{MediaType: "image/jpeg", Data: png.Data}}}); err == nil {
		t.Error("a PNG sent as a JPEG was taken")
	}
	if _, err := e.PromptWith(ctx, ses.ID, PromptRequest{Text: "what is this?", Images: []ImageInput{png}}); err != nil {
		t.Fatal(err)
	}
	waitIdle(t, ch, ses.ID)
	unsub()

	sent := p.Requests()[0].Messages
	img := sent[len(sent)-1].Images()
	if len(img) != 1 || !bytes.Equal(img[0].Data, tinyPNG) || img[0].MediaType != "image/png" {
		t.Fatalf("the model got %+v", sent[len(sent)-1])
	}
	evs, _ := e.Events(ctx, ses.ID, 0, 0)
	var block event.ContentBlock
	for _, ev := range evs {
		if ev.Type == event.UserMessageChunk && strings.Contains(string(ev.Data), `"image"`) {
			var d event.MessageChunk
			if err := json.Unmarshal(ev.Data, &d); err != nil {
				t.Fatal(err)
			}
			block = d.Content
		}
		if bytes.Contains(ev.Data, []byte(png.Data[:40])) {
			t.Errorf("%s event carries the picture's bytes", ev.Type)
		}
	}
	name := strings.TrimPrefix(block.URI, "attachment:")
	if block.Type != "image" || block.MimeType != "image/png" || name == block.URI {
		t.Fatalf("image block = %+v", block)
	}
	if a, err := e.Attachment(ctx, ses.ID, name); err != nil || a.Data != png.Data {
		t.Errorf("attachment = %+v, %v", a, err)
	}
	if md, _ := e.RenderExport(ctx, ses.ID, "markdown"); !strings.Contains(md, "*Attached picture: "+name+"*") {
		t.Errorf("the Markdown export does not name the picture:\n%s", md)
	}
	e.Close()

	// After a restart the history names the picture; the next request has
	// its bytes again.
	e = openTest(t, dir)
	defer e.Close()
	p2 := scripted.New("sp", scripted.Step{Text: "still red"})
	e.mu.Lock()
	e.providers.built["sp"] = &configuredProvider{cfg: config.Provider{ID: "sp", Name: "sp", Type: "scripted"}, p: p2}
	e.mu.Unlock()
	ch, unsub = e.Subscribe()
	defer unsub()
	if _, err := e.Prompt(ctx, ses.ID, "and now?"); err != nil {
		t.Fatal(err)
	}
	waitIdle(t, ch, ses.ID)
	var found []*provider.Image
	for _, m := range p2.Requests()[0].Messages {
		found = append(found, m.Images()...)
	}
	if len(found) != 1 || !bytes.Equal(found[0].Data, tinyPNG) {
		t.Errorf("after a restart the model got %d images", len(found))
	}
}
