package event

import (
	"encoding/json"
	"os"
	"slices"
	"testing"
)

type schemaDoc struct {
	Properties struct {
		Type struct {
			Enum     []Type `json:"enum"`
			ACPTypes []Type `json:"x-acp-types"`
		} `json:"type"`
		Data struct {
			Type string `json:"type"`
		} `json:"data"`
	} `json:"properties"`
	Defs struct {
		SessionStatus struct {
			Properties struct {
				Status struct {
					Enum []string `json:"enum"`
				} `json:"status"`
			} `json:"properties"`
		} `json:"sessionStatus"`
	} `json:"$defs"`
}

func loadSchema(t *testing.T) schemaDoc {
	t.Helper()
	b, err := os.ReadFile("../../schemas/events.json")
	if err != nil {
		t.Fatal(err)
	}
	var s schemaDoc
	if err := json.Unmarshal(b, &s); err != nil {
		t.Fatalf("schemas/events.json is not valid JSON: %v", err)
	}
	return s
}

func TestSchemaTypesMatchGo(t *testing.T) {
	s := loadSchema(t)
	if got, want := s.Properties.Type.Enum, AllTypes(); !slices.Equal(got, want) {
		t.Errorf("schema type enum = %v\nGo AllTypes()   = %v", got, want)
	}
	if got := s.Properties.Type.ACPTypes; !slices.Equal(got, ACPTypes) {
		t.Errorf("schema x-acp-types = %v\nGo ACPTypes        = %v", got, ACPTypes)
	}
}

func TestEnvelopeFieldNames(t *testing.T) {
	b, err := json.Marshal(Event{V: Version, Seq: 1, SID: "s", Type: Notice, Data: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"v", "seq", "ts", "sid", "type", "data"} {
		if _, ok := m[k]; !ok {
			t.Errorf("marshalled event lacks %q: %s", k, b)
		}
	}
	if len(m) != 6 {
		t.Errorf("marshalled event has extra fields: %s", b)
	}
}

func TestValid(t *testing.T) {
	if !AgentMessageChunk.Valid() || !TurnEnded.Valid() {
		t.Error("known types reported invalid")
	}
	if Type("nope").Valid() {
		t.Error("unknown type reported valid")
	}
}
