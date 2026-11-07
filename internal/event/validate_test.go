package event

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

func schemaValidator(t *testing.T) *Validator {
	t.Helper()
	b, err := os.ReadFile("../../schemas/events.json")
	if err != nil {
		t.Fatal(err)
	}
	v, err := NewValidator(b)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

// EXT-002: the validator passes what the app writes and catches what breaks the schema,
// so the store's check in tests (FREELOOP_CHECK_EVENTS) means something.
func TestValidatorCatchesWhatTheSchemaRefuses(t *testing.T) {
	v := schemaValidator(t)
	ev := func(typ Type, data string) Event {
		return Event{V: Version, Seq: 1, SID: "ses_1", TS: time.Now().UTC(), Type: typ, Data: json.RawMessage(data)}
	}
	for _, good := range []Event{
		ev(SessionStatus, `{"status":"running"}`),
		ev(AgentMessageChunk, `{"content":{"type":"text","text":"hi"}}`),
		ev(UsageUpdate, `{"inputTokens":3,"outputTokens":1,"cacheReadTokens":0,"cacheWriteTokens":0,"measured":"turn"}`),
		ev(TurnEnded, `{"stopReason":"end_turn"}`),
	} {
		if err := v.Validate(good); err != nil {
			t.Errorf("a good event was refused: %v", err)
		}
	}
	for want, bad := range map[string]Event{
		"not one of":  ev(SessionStatus, `{"status":"sleeping"}`),
		"not of type": ev(UsageUpdate, `{"inputTokens":"three"}`),
		"type":        ev(Type("made_up"), `{}`),
		"measured":    ev(UsageUpdate, `{"measured":"guessed"}`),
	} {
		err := v.Validate(bad)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: %v", want, err)
		}
	}
}
