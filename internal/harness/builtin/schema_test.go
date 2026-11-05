package builtin

import (
	"encoding/json"
	"fmt"
	"testing"
)

// Strict endpoints check a tool's schema and refuse the whole request over one mistake:
// Azure answered 400 for list_dir's "required": null. So every tool's schema, as it goes on
// the wire, has to be plain JSON Schema.
func TestToolSchemasSurviveAStrictEndpoint(t *testing.T) {
	list := tools()
	if len(list) == 0 {
		t.Fatal("no tools")
	}
	for _, tool := range list {
		raw, err := json.Marshal(tool.spec.InputSchema)
		if err != nil {
			t.Fatalf("%s: %v", tool.spec.Name, err)
		}
		var v any
		if err := json.Unmarshal(raw, &v); err != nil {
			t.Fatalf("%s: %v", tool.spec.Name, err)
		}
		for _, problem := range schemaProblems(v, "") {
			t.Errorf("%s: %s in %s", tool.spec.Name, problem, raw)
		}
		top, _ := v.(map[string]any)
		if _, ok := top["required"].([]any); !ok {
			t.Errorf("%s: required is not an array: %s", tool.spec.Name, raw)
		}
		if _, ok := top["properties"].(map[string]any); !ok {
			t.Errorf("%s: properties is not an object: %s", tool.spec.Name, raw)
		}
	}
}

func schemaProblems(v any, at string) []string {
	var out []string
	switch x := v.(type) {
	case nil:
		out = append(out, fmt.Sprintf("null at %q", at))
	case map[string]any:
		if x["type"] == "array" && x["items"] == nil {
			out = append(out, fmt.Sprintf("an array without items at %q", at))
		}
		for k, sub := range x {
			out = append(out, schemaProblems(sub, at+"."+k)...)
		}
	case []any:
		for i, sub := range x {
			out = append(out, schemaProblems(sub, fmt.Sprintf("%s[%d]", at, i))...)
		}
	}
	return out
}

func TestASchemaWithNothingRequiredSaysSo(t *testing.T) {
	raw, err := json.Marshal(schema(nil, map[string]any{"path": str("A path")}))
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Required *[]string `json:"required"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got.Required == nil || len(*got.Required) != 0 {
		t.Fatalf("required is not an empty array: %s", raw)
	}
}
