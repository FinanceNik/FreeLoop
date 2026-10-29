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
	ID          string `json:"id"`
	Title       string `json:"title"`
	Status      Status `json:"status"`
	HarnessID   string `json:"harnessId"`
	ProjectID   string `json:"projectId"`
	ProjectRoot string `json:"projectRoot"`
	// EnvironmentID names the environment the session runs with (ENV-003).
	EnvironmentID string `json:"environmentId"`
	// BaseCommit is HEAD when the session started, for the diff panel;
	// Branch is the latest branch seen (SES-006).
	BaseCommit string `json:"baseCommit"`
	Branch     string `json:"branch"`
	ProviderID string `json:"providerId"`
	Model      string `json:"model"`
	Mode       string `json:"mode"`
	// ProfileID names the harness profile the session runs with (HRN-003).
	ProfileID string    `json:"profileId"`
	LastSeq   int64     `json:"lastSeq"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// Project is a repository the user works in (ENV-003). It is identified by
// its path plus a fingerprint of its first commit and remote URL, so a moved
// checkout can be relinked (OQ-020).
type Project struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Root        string `json:"root"`
	FirstCommit string `json:"firstCommit"`
	Remote      string `json:"remote"`
	// DefaultEnvironmentID is preselected for new sessions.
	DefaultEnvironmentID string `json:"defaultEnvironmentId"`
	// DefaultProfileID is preselected for new sessions (HRN-004).
	DefaultProfileID string `json:"defaultProfileId"`
	// Trusted is set once the user trusts the project (SEC-006); until
	// then the app reads none of its instruction files or .freeloop/
	// settings and runs no git command that its config can extend.
	Trusted bool `json:"trusted"`
	// Muted projects send no notifications (GUI-008).
	Muted bool `json:"muted"`
	// DisabledSkills are skills turned off in this project (SKL-002).
	DisabledSkills []string  `json:"disabledSkills"`
	CreatedAt      time.Time `json:"createdAt"`
}

var idEncoding = base32.NewEncoding("0123456789abcdefghjkmnpqrstvwxyz").WithPadding(base32.NoPadding)

// NewID returns a time-sortable session ID: "ses_" followed by 48 bits of
// millisecond time and 80 random bits in Crockford base32.
func NewID(now time.Time) string { return newID("ses_", now) }

// NewProjectID is NewID for projects, with the prefix "prj_".
func NewProjectID(now time.Time) string { return newID("prj_", now) }

func newID(prefix string, now time.Time) string {
	var b [16]byte
	binary.BigEndian.PutUint64(b[:8], uint64(now.UnixMilli())<<16)
	rand.Read(b[6:])
	return prefix + strings.ToLower(idEncoding.EncodeToString(b[:]))
}
