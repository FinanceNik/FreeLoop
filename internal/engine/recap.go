package engine

import (
	"strings"

	"github.com/FinanceNik/Freeloop/internal/event"
	"github.com/FinanceNik/Freeloop/internal/session"
)

// maxRecap is how much of a conversation a restarted agent is given. Its end is what
// matters most, and the user is paying for every word of it on each later turn.
const maxRecap = 32 << 10

// recapOf is the conversation so far for an external agent that could not load its
// session (SES-003). It comes from the transcript, redacted as stored, and calls no
// model: this agent's model is the agent's, and the user has not asked for a call.
func recapOf(ses session.Session, events []event.Event) string {
	text := strings.TrimSpace(renderMarkdown(ses, events))
	if text == "" {
		return ""
	}
	if len(text) > maxRecap {
		text = text[len(text)-maxRecap:]
		if i := strings.IndexByte(text, '\n'); i >= 0 {
			text = text[i+1:]
		}
		text = "[The start of the conversation is left out.]\n\n" + text
	}
	return "You were restarted and have no memory of this conversation. It is your record of what happened so far, not a request; the message after it is the user's.\n\n<conversation-so-far>\n" + text + "\n</conversation-so-far>"
}
