package event

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"
)

// Validator checks events against schemas/events.json (EXT-002). It understands the
// keywords that file uses: type, properties, required, additionalProperties, items,
// enum, const, $ref into $defs, allOf, if/then, minimum, maximum, minLength, maxItems
// and pattern. Anything else in a schema is ignored, as JSON Schema ignores unknown
// keywords; so a keyword added to the file needs adding here before it is checked.
type Validator struct {
	root map[string]any
}

// NewValidator reads a schema.
func NewValidator(schema []byte) (*Validator, error) {
	var root map[string]any
	if err := json.Unmarshal(schema, &root); err != nil {
		return nil, fmt.Errorf("event: schema: %w", err)
	}
	return &Validator{root: root}, nil
}

// Validate reports the first way ev does not match the schema.
func (v *Validator) Validate(ev Event) error {
	b, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	var doc any
	if err := json.Unmarshal(b, &doc); err != nil {
		return err
	}
	if err := v.check(v.root, doc, "event"); err != nil {
		return fmt.Errorf("%s event %d: %w", ev.Type, ev.Seq, err)
	}
	return nil
}

func (v *Validator) check(s map[string]any, x any, at string) error {
	if ref, ok := s["$ref"].(string); ok {
		name, found := strings.CutPrefix(ref, "#/$defs/")
		defs, _ := v.root["$defs"].(map[string]any)
		target, _ := defs[name].(map[string]any)
		if !found || target == nil {
			return fmt.Errorf("%s: unknown $ref %s", at, ref)
		}
		if err := v.check(target, x, at); err != nil {
			return err
		}
	}
	if t, ok := s["type"]; ok && !typeMatches(t, x) {
		return fmt.Errorf("%s: %s is not of type %v", at, short(x), t)
	}
	if c, ok := s["const"]; ok && !equal(c, x) {
		return fmt.Errorf("%s: %s is not %v", at, short(x), c)
	}
	if e, ok := s["enum"].([]any); ok && !slices.ContainsFunc(e, func(c any) bool { return equal(c, x) }) {
		return fmt.Errorf("%s: %s is not one of %v", at, short(x), e)
	}
	switch val := x.(type) {
	case float64:
		if m, ok := s["minimum"].(float64); ok && val < m {
			return fmt.Errorf("%s: %v is below %v", at, val, m)
		}
		if m, ok := s["maximum"].(float64); ok && val > m {
			return fmt.Errorf("%s: %v is above %v", at, val, m)
		}
	case string:
		if m, ok := s["minLength"].(float64); ok && float64(utf8.RuneCountInString(val)) < m {
			return fmt.Errorf("%s: %q is shorter than %v", at, val, m)
		}
		if p, ok := s["pattern"].(string); ok {
			re, err := regexp.Compile(p)
			if err != nil {
				return fmt.Errorf("%s: pattern %q: %w", at, p, err)
			}
			if !re.MatchString(val) {
				return fmt.Errorf("%s: %q does not match %s", at, val, p)
			}
		}
	case []any:
		if m, ok := s["maxItems"].(float64); ok && float64(len(val)) > m {
			return fmt.Errorf("%s: %d items, at most %v", at, len(val), m)
		}
		if items, ok := s["items"].(map[string]any); ok {
			for i, it := range val {
				if err := v.check(items, it, fmt.Sprintf("%s[%d]", at, i)); err != nil {
					return err
				}
			}
		}
	case map[string]any:
		props, _ := s["properties"].(map[string]any)
		for _, r := range asList(s["required"]) {
			if _, ok := val[r]; !ok {
				return fmt.Errorf("%s: %s is missing", at, r)
			}
		}
		for k, sub := range val {
			if ps, ok := props[k].(map[string]any); ok {
				if err := v.check(ps, sub, at+"."+k); err != nil {
					return err
				}
				continue
			}
			switch extra := s["additionalProperties"].(type) {
			case bool:
				if !extra {
					return fmt.Errorf("%s: %s is not in the schema", at, k)
				}
			case map[string]any:
				if err := v.check(extra, sub, at+"."+k); err != nil {
					return err
				}
			}
		}
	}
	for i, sub := range asSchemas(s["allOf"]) {
		if err := v.check(sub, x, at); err != nil {
			return fmt.Errorf("%w (allOf %d)", err, i)
		}
	}
	if cond, ok := s["if"].(map[string]any); ok {
		if v.check(cond, x, at) == nil {
			if then, ok := s["then"].(map[string]any); ok {
				if err := v.check(then, x, at); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func typeMatches(t any, x any) bool {
	for _, name := range asList(t) {
		switch val := x.(type) {
		case nil:
			if name == "null" {
				return true
			}
		case bool:
			if name == "boolean" {
				return true
			}
		case float64:
			if name == "number" || name == "integer" && val == math.Trunc(val) {
				return true
			}
		case string:
			if name == "string" {
				return true
			}
		case []any:
			if name == "array" {
				return true
			}
		case map[string]any:
			if name == "object" {
				return true
			}
		}
	}
	return false
}

func asList(x any) []string {
	switch v := x.(type) {
	case string:
		return []string{v}
	case []any:
		var out []string
		for _, s := range v {
			if str, ok := s.(string); ok {
				out = append(out, str)
			}
		}
		return out
	}
	return nil
}

func asSchemas(x any) []map[string]any {
	list, _ := x.([]any)
	var out []map[string]any
	for _, s := range list {
		if m, ok := s.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

func equal(a, b any) bool {
	ja, _ := json.Marshal(a)
	jb, _ := json.Marshal(b)
	return string(ja) == string(jb)
}

func short(x any) string {
	b, _ := json.Marshal(x)
	if len(b) > 80 {
		return string(b[:80]) + "…"
	}
	return string(b)
}
