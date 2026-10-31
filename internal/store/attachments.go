package store

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
)

// Attachments are pictures a user attached to a session's messages
// (SES-015), one file per content in sessions/<id>/attachments/, named by
// its SHA-256, so the same picture twice is stored once and transcripts
// refer to it by name instead of carrying its bytes.

// MaxAttachment caps one attachment.
const MaxAttachment = 5 << 20

var attachmentExt = map[string]string{"image/png": "png", "image/jpeg": "jpg", "image/gif": "gif", "image/webp": "webp"}

var attachmentName = regexp.MustCompile(`^[0-9a-f]{64}\.(png|jpg|gif|webp)$`)

// AttachmentType returns the media type a stored attachment's name implies.
func AttachmentType(name string) string {
	for t, ext := range attachmentExt {
		if filepath.Ext(name) == "."+ext {
			return t
		}
	}
	return ""
}

func (s *Store) attachmentsDir(sid string) string {
	return filepath.Join(s.sessionDir(sid), "attachments")
}

// SaveAttachment stores data of the given media type for a session and
// returns its name.
func (s *Store) SaveAttachment(sid, mediaType string, data []byte) (string, error) {
	ext, ok := attachmentExt[mediaType]
	if !ok {
		return "", fmt.Errorf("%s is not an image type the app keeps", mediaType)
	}
	if len(data) == 0 || len(data) > MaxAttachment {
		return "", fmt.Errorf("an image must be under %d MB", MaxAttachment>>20)
	}
	sum := sha256.Sum256(data)
	name := hex.EncodeToString(sum[:]) + "." + ext
	dir := s.attachmentsDir(sid)
	path := filepath.Join(dir, name)
	if _, err := os.Stat(path); err == nil {
		return name, nil
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("store: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return "", fmt.Errorf("store: %w", err)
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return "", fmt.Errorf("store: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	return name, os.Rename(tmp.Name(), path)
}

// Attachment reads a stored attachment by name.
func (s *Store) Attachment(sid, name string) ([]byte, error) {
	if !attachmentName.MatchString(name) {
		return nil, errors.New("not an attachment name")
	}
	f, err := os.Open(filepath.Join(s.attachmentsDir(sid), name))
	if err != nil {
		return nil, fmt.Errorf("store: %w", err)
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, MaxAttachment+1))
}

// CopyAttachments copies a session's attachments to another, for a fork.
func (s *Store) CopyAttachments(from, to string) error {
	entries, err := os.ReadDir(s.attachmentsDir(from))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, e := range entries {
		if !attachmentName.MatchString(e.Name()) {
			continue
		}
		data, err := s.Attachment(from, e.Name())
		if err != nil {
			return err
		}
		if _, err := s.SaveAttachment(to, AttachmentType(e.Name()), data); err != nil {
			return err
		}
	}
	return nil
}
