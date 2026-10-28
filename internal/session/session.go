// Package session holds the engine-side session record the GUI lists. The
// SessionManager that runs sessions lands here in M1.
package session

import (
	"crypto/rand"
	"encoding/base32"
	"encoding/binary"
	"strings"
	"time"
)

// Status is a session's coarse state, shown in the sidebar.
type Status string

const (
	StatusIdle              Status = "idle"
	StatusRunning           Status = "running"
	StatusWaitingPermission Status = "waiting_permission"
	StatusWaitingInput      Status = "waiting_input"
	StatusError             Status = "error"
	StatusArchived          Status = "archived"
)

// Session is the persisted record of one session. Fields from the architecture
// sketch that nothing uses yet (project, environment, profile, model, git) are
// added with the milestone that needs them.
type Session struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Status      Status    `json:"status"`
	HarnessID   string    `json:"harnessId"`
	ProjectRoot string    `json:"projectRoot"`
	ProviderID  string    `json:"providerId"`
	Model       string    `json:"model"`
	Mode        string    `json:"mode"`
	LastSeq     int64     `json:"lastSeq"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

var idEncoding = base32.NewEncoding("0123456789abcdefghjkmnpqrstvwxyz").WithPadding(base32.NoPadding)

// NewID returns a time-sortable session ID: "ses_" followed by 48 bits of
// millisecond time and 80 random bits in Crockford base32.
func NewID(now time.Time) string {
	var b [16]byte
	binary.BigEndian.PutUint64(b[:8], uint64(now.UnixMilli())<<16)
	rand.Read(b[6:])
	return "ses_" + strings.ToLower(idEncoding.EncodeToString(b[:]))
}
