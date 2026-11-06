package harness

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestQuestionsCheck(t *testing.T) {
	ok := []Question{{Question: "Which?", Options: []QuestionOption{{Label: "A"}, {Label: "B"}}}}
	if err := CheckQuestions(ok); err != nil {
		t.Fatal(err)
	}
	for name, qs := range map[string][]Question{
		"none":           nil,
		"five":           {ok[0], ok[0], ok[0], ok[0], ok[0]},
		"no text":        {{Question: " ", Options: ok[0].Options}},
		"one option":     {{Question: "Which?", Options: []QuestionOption{{Label: "A"}}}},
		"same label":     {{Question: "Which?", Options: []QuestionOption{{Label: "A"}, {Label: "A"}}}},
		"label is blank": {{Question: "Which?", Options: []QuestionOption{{Label: "A"}, {Label: ""}}}},
	} {
		if CheckQuestions(qs) == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestQuestionFormAndAnswers(t *testing.T) {
	qs := []Question{
		{Question: "How should I estimate?", Header: "Mode", Options: []QuestionOption{{Label: "All at once", Description: "Paste all first"}, {Label: "One by one"}}},
		{Question: "Which checks?", Options: []QuestionOption{{Label: "Tests"}, {Label: "Lint"}}, MultiSelect: true},
	}
	message, raw := QuestionForm(qs)
	if message != "Please answer the following questions." {
		t.Errorf("message = %q", message)
	}
	var schema struct {
		Properties map[string]map[string]any `json:"properties"`
	}
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatal(err)
	}
	first := schema.Properties["question_0"]
	if first["title"] != "Mode" || first["description"] != "How should I estimate?" || !strings.Contains(string(raw), `"description":"Paste all first"`) {
		t.Errorf("question_0 = %v", first)
	}
	if schema.Properties["question_1"]["type"] != "array" || schema.Properties["question_1_custom"]["type"] != "string" {
		t.Errorf("schema = %s", raw)
	}
	if one, _ := QuestionForm(qs[:1]); one != "How should I estimate?" {
		t.Errorf("a single question's message = %q", one)
	}

	got := QuestionAnswers(qs, map[string]any{"question_0": "One by one", "question_0_custom": "start with the backend", "question_1": []any{"Tests"}, "question_1_custom": "Types"})
	want := `"How should I estimate?" = "One by one (note: start with the backend)"` + "\n" + `"Which checks?" = "Tests, Types"`
	if got != want {
		t.Errorf("answers =\n%s\nwant\n%s", got, want)
	}
	if got := QuestionAnswers(qs[:1], map[string]any{"question_0_custom": "neither"}); got != `"How should I estimate?" = "neither"` {
		t.Errorf("own answer = %s", got)
	}
	if got := QuestionAnswers(qs[:1], nil); !strings.Contains(got, "(skipped)") {
		t.Errorf("skipped = %s", got)
	}
}
