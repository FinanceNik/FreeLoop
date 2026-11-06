package harness

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// Questions an agent asks the user (HRN-021), each with options to choose from and room
// for an answer of the user's own. The form has the layout Claude Code's ACP adapter
// sends for its AskUserQuestion tool, so the app shows both agents' questions alike:
// question_<n> to choose in, question_<n>_custom for the user's own words.

// Question is one question, as the agent asks it.
type Question struct {
	Question    string           `json:"question"`
	Header      string           `json:"header,omitempty"`
	Options     []QuestionOption `json:"options"`
	MultiSelect bool             `json:"multiSelect,omitempty"`
}

// QuestionOption is one answer to choose.
type QuestionOption struct {
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}

// Limits on what one call may ask: enough for a real choice, too few to bury the user.
const (
	maxQuestions = 4
	maxOptions   = 8
)

// CheckQuestions reports what is wrong with questions an agent sent.
func CheckQuestions(qs []Question) error {
	if len(qs) == 0 || len(qs) > maxQuestions {
		return fmt.Errorf("ask 1 to %d questions at a time", maxQuestions)
	}
	for i, q := range qs {
		if strings.TrimSpace(q.Question) == "" {
			return fmt.Errorf("question %d has no text", i+1)
		}
		if len(q.Options) < 2 || len(q.Options) > maxOptions {
			return fmt.Errorf("question %d needs 2 to %d options", i+1, maxOptions)
		}
		seen := map[string]bool{}
		for _, o := range q.Options {
			label := strings.TrimSpace(o.Label)
			if label == "" || seen[label] {
				return errors.New("each option needs a label of its own")
			}
			seen[label] = true
		}
	}
	return nil
}

func questionField(i int) string { return fmt.Sprintf("question_%d", i) }
func customField(i int) string   { return fmt.Sprintf("question_%d_custom", i) }

// QuestionForm is the form for questions: its message and JSON Schema.
func QuestionForm(qs []Question) (string, json.RawMessage) {
	props := map[string]any{}
	for i, q := range qs {
		var options []map[string]any
		for _, o := range q.Options {
			opt := map[string]any{"const": o.Label, "title": o.Label}
			if o.Description != "" {
				opt["description"] = o.Description
			}
			options = append(options, opt)
		}
		field := map[string]any{"type": "string", "oneOf": options}
		if q.MultiSelect {
			field = map[string]any{"type": "array", "items": map[string]any{"anyOf": options}}
		}
		if q.Header != "" {
			field["title"] = q.Header
		}
		if len(qs) > 1 {
			field["description"] = q.Question
		}
		props[questionField(i)] = field
		own := "Type your own answer, or add a note to the option you chose (optional)."
		if q.MultiSelect {
			own = "Type your own answer to add to your selection (optional)."
		}
		props[customField(i)] = map[string]any{"type": "string", "title": "Other", "description": own}
	}
	message := "Please answer the following questions."
	if len(qs) == 1 {
		message = qs[0].Question
	}
	schema, _ := json.Marshal(map[string]any{"type": "object", "properties": props})
	return message, schema
}

// QuestionAnswers says what the user answered, for the agent: one line per question.
// Nothing chosen and nothing typed is a question the user skipped.
func QuestionAnswers(qs []Question, content map[string]any) string {
	var b strings.Builder
	for i, q := range qs {
		var picks []string
		switch v := content[questionField(i)].(type) {
		case string:
			if v != "" {
				picks = append(picks, v)
			}
		case []any:
			for _, x := range v {
				if s, ok := x.(string); ok && s != "" {
					picks = append(picks, s)
				}
			}
		}
		own, _ := content[customField(i)].(string)
		own = strings.TrimSpace(own)
		answer := strings.Join(picks, ", ")
		switch {
		case answer == "" && own == "":
			answer = "(skipped)"
		case answer == "":
			answer = own
		case own != "" && q.MultiSelect:
			answer += ", " + own
		case own != "":
			answer += " (note: " + own + ")"
		}
		fmt.Fprintf(&b, "%q = %q\n", q.Question, answer)
	}
	return strings.TrimRight(b.String(), "\n")
}
