package web

import (
	"strings"
	"testing"
)

func TestText(t *testing.T) {
	title, text := Text(`<!doctype html><html><head><title>A &amp; B</title><style>p{color:red}</style></head>
<body><nav>Menu</nav><script>alert("hidden")</script><!-- gone -->
<h1>Heading</h1><p>First   <b>bold</b> line.</p><ul><li>one</li><li>two</li></ul><p>&lt;tag&gt; stays text</p></body></html>`)
	if title != "A & B" {
		t.Errorf("title = %q", title)
	}
	for _, want := range []string{"Heading", "First bold line.", "one\n", "two", "<tag> stays text"} {
		if !strings.Contains(text, want) {
			t.Errorf("text lacks %q:\n%s", want, text)
		}
	}
	for _, not := range []string{"alert", "color:red", "gone", "<h1>"} {
		if strings.Contains(text, not) {
			t.Errorf("text has %q:\n%s", not, text)
		}
	}
}

func TestParseResults(t *testing.T) {
	sx, err := ParseResults([]byte(`{"results":[{"title":"T","url":"https://a","content":"<b>S</b>nip"},{"title":"no url"}]}`))
	if err != nil || len(sx) != 1 || sx[0].Snippet != "Snip" {
		t.Errorf("searxng = %+v, %v", sx, err)
	}
	br, err := ParseResults([]byte(`{"web":{"results":[{"title":"B","url":"https://b","description":"D"}]}}`))
	if err != nil || len(br) != 1 || br[0].Snippet != "D" {
		t.Errorf("brave = %+v, %v", br, err)
	}
	if _, err := ParseResults([]byte("<html>")); err == nil {
		t.Error("HTML was read as results")
	}
}
