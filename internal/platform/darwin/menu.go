package darwin

// MenuWords are a Mac's own names for menu items where other systems say otherwise:
// its File menu is Ablage in German and Archief in Dutch (GUI-048).
var MenuWords = map[string]map[string]string{
	"de": {"File": "Ablage"},
	"nl": {"File": "Archief"},
}
