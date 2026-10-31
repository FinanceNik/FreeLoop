package engine

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"

	"github.com/FinanceNik/Freeloop/internal/provider"
	"github.com/FinanceNik/Freeloop/internal/store"
)

// Images in prompts (SES-015). The page scales pictures down before it
// sends them; the engine checks what arrives and stores each as one of the
// session's attachments, so a transcript names its pictures instead of
// carrying them.

// MaxImages is how many pictures one message takes.
const MaxImages = 10

// ImageInput is one picture of a message, base64 encoded.
type ImageInput struct {
	MediaType string `json:"mediaType"`
	Data      string `json:"data"`
}

// storeImages checks and stores a message's pictures.
func (e *Engine) storeImages(ctx context.Context, sid string, in []ImageInput) ([]provider.Image, error) {
	if len(in) == 0 {
		return nil, nil
	}
	if len(in) > MaxImages {
		return nil, fmt.Errorf("a message takes at most %d images", MaxImages)
	}
	if _, err := e.store.GetSession(ctx, sid); err != nil {
		return nil, err
	}
	out := make([]provider.Image, 0, len(in))
	for i, img := range in {
		data, err := base64.StdEncoding.DecodeString(img.Data)
		if err != nil {
			return nil, fmt.Errorf("image %d is not base64", i+1)
		}
		// What the bytes are, not what the page says they are.
		if got := http.DetectContentType(data); got != img.MediaType {
			return nil, fmt.Errorf("image %d is %s, not %s", i+1, got, img.MediaType)
		}
		name, err := e.store.SaveAttachment(sid, img.MediaType, data)
		if err != nil {
			return nil, fmt.Errorf("image %d: %w", i+1, err)
		}
		out = append(out, provider.Image{Name: name, MediaType: img.MediaType, Data: data})
	}
	return out, nil
}

// Attachment is a stored picture for the page to show.
type Attachment struct {
	MediaType string `json:"mediaType"`
	Data      string `json:"data"`
}

// Attachment returns one of a session's stored pictures.
func (e *Engine) Attachment(ctx context.Context, sid, name string) (Attachment, error) {
	if _, err := e.store.GetSession(ctx, sid); err != nil {
		return Attachment{}, err
	}
	data, err := e.store.Attachment(sid, name)
	if err != nil {
		return Attachment{}, err
	}
	return Attachment{MediaType: store.AttachmentType(name), Data: base64.StdEncoding.EncodeToString(data)}, nil
}
