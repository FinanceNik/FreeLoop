package main

import (
	"fmt"
	"strings"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/engine"
)

// The desktop layer's own words (GUI-048): the menu bar's and the system
// notifications', in the interface's language. The window says which language that
// is (EngineService.SetInterfaceLanguage), so "system" is resolved once, as the
// window resolves it. The engine's notices stay English and say what happened in
// fields, from which these are written; a kind not known here shows the engine's
// text.

// interfaceLanguages are the languages there are words for; English is the source.
var interfaceLanguages = []string{"en", "de", "nl"}

// menuWords are the menu items' names, by their English ones. A system's own names,
// where they differ, are in its platform's MenuWords (a Mac's File menu is Ablage).
var menuWords = map[string]map[string]string{
	"de": {
		"File": "Datei", "Edit": "Bearbeiten", "View": "Darstellung", "Window": "Fenster", "Help": "Hilfe",
		"About Freeloop": "Über Freeloop", "Services": "Dienste", "Hide Freeloop": "Freeloop ausblenden",
		"Hide Others": "Andere ausblenden", "Show All": "Alle einblenden", "Quit Freeloop": "Freeloop beenden",
		"Close": "Schließen", "Undo": "Widerrufen", "Redo": "Wiederholen", "Cut": "Ausschneiden", "Copy": "Kopieren",
		"Paste": "Einsetzen", "Paste and Match Style": "Einsetzen und Stil anpassen", "Delete": "Löschen",
		"Select All": "Alles auswählen", "Speech": "Sprachausgabe", "Start Speaking": "Sprachausgabe starten",
		"Stop Speaking": "Sprachausgabe stoppen", "Reload": "Neu laden", "Force Reload": "Neu laden erzwingen",
		"Open Developer Tools": "Entwicklerwerkzeuge öffnen", "Toggle Full Screen": "Vollbildmodus ein/aus",
		"Minimize": "Im Dock ablegen", "Zoom": "Zoomen", "Bring All to Front": "Alle nach vorne bringen",
		"Freeloop Help": "Freeloop-Hilfe",
	},
	"nl": {
		"File": "Bestand", "Edit": "Wijzig", "View": "Weergave", "Window": "Venster", "Help": "Help",
		"About Freeloop": "Over Freeloop", "Services": "Voorzieningen", "Hide Freeloop": "Verberg Freeloop",
		"Hide Others": "Verberg andere", "Show All": "Toon alles", "Quit Freeloop": "Stop Freeloop",
		"Close": "Sluit", "Undo": "Herstel", "Redo": "Opnieuw", "Cut": "Knip", "Copy": "Kopieer",
		"Paste": "Plak", "Paste and Match Style": "Plak en pas stijl aan", "Delete": "Verwijder",
		"Select All": "Selecteer alles", "Speech": "Spraak", "Start Speaking": "Start met spreken",
		"Stop Speaking": "Stop met spreken", "Reload": "Laad opnieuw", "Force Reload": "Forceer opnieuw laden",
		"Open Developer Tools": "Open ontwikkelaarstools", "Toggle Full Screen": "Schermvullende weergave aan/uit",
		"Minimize": "Minimaliseer", "Zoom": "Zoom", "Bring All to Front": "Breng alles naar voren",
		"Freeloop Help": "Freeloop Help",
	},
}

// localizeMenu names every item of m in lang, each by its English name; one with no
// word here keeps its name. The menu is built in English each time, so a change of
// language starts from the source.
func localizeMenu(m *application.Menu, lang string, system map[string]map[string]string) {
	for i := 0; ; i++ {
		item := m.ItemAt(i)
		if item == nil {
			return
		}
		if word, ok := system[lang][item.Label()]; ok {
			item.SetLabel(word)
		} else if word, ok := menuWords[lang][item.Label()]; ok {
			item.SetLabel(word)
		}
		if sub := item.GetSubmenu(); sub != nil {
			localizeMenu(sub, lang, system)
		}
	}
}

// noticeWords are the notifications' words, by language.
var noticeWords = map[string]map[string]string{
	"en": {
		"approval": "Approval needed: %s", "error": "Session failed: %s", "finished": "Session finished: %s",
		"end_turn": "Finished", "max_tokens": "Stopped at the output limit", "max_turns": "Stopped at the turn limit",
		"refusal": "The model declined the request",
		"warning": "Budget nearly spent", "reached": "Budget reached",
		"day": "The daily budget of %s", "month": "The monthly budget of %s", "profile": "%s for profile %s",
		"spentWarning": "%s: %s spent so far, %s%%.", "spentReached": "%s: %s spent so far. Running sessions go on; the queue waits.",
	},
	"de": {
		"approval": "Freigabe nötig: %s", "error": "Sitzung fehlgeschlagen: %s", "finished": "Sitzung beendet: %s",
		"end_turn": "Fertig", "max_tokens": "Angehalten: Ausgabelimit erreicht", "max_turns": "Angehalten: Rundenlimit erreicht",
		"refusal": "Das Modell hat die Anfrage abgelehnt",
		"warning": "Kostenrahmen fast ausgeschöpft", "reached": "Kostenrahmen erreicht",
		"day": "Der tägliche Kostenrahmen von %s", "month": "Der monatliche Kostenrahmen von %s", "profile": "%s für das Profil %s",
		"spentWarning": "%s: bisher %s ausgegeben, %s %%.", "spentReached": "%s: bisher %s ausgegeben. Laufende Sitzungen laufen weiter; die Warteschlange wartet.",
	},
	"nl": {
		"approval": "Goedkeuring nodig: %s", "error": "Sessie mislukt: %s", "finished": "Sessie klaar: %s",
		"end_turn": "Klaar", "max_tokens": "Gestopt: uitvoerlimiet bereikt", "max_turns": "Gestopt: beurtlimiet bereikt",
		"refusal": "Het model heeft het verzoek geweigerd",
		"warning": "Budget bijna op", "reached": "Budget bereikt",
		"day": "Het dagbudget van %s", "month": "Het maandbudget van %s", "profile": "%s voor profiel %s",
		"spentWarning": "%s: tot nu toe %s uitgegeven, %s%%.", "spentReached": "%s: tot nu toe %s uitgegeven. Lopende sessies gaan door; de wachtrij wacht.",
	},
}

// noticeText is n's title and body in lang. A kind, stop reason or language it has no
// words for keeps the engine's English.
func noticeText(n engine.Notice, lang string) (title, body string) {
	w, ok := noticeWords[lang]
	if !ok {
		return n.Title, n.Body
	}
	switch n.Kind {
	case "approval", "error":
		return fmt.Sprintf(w[n.Kind], n.Session), n.Detail
	case "finished":
		if reason, ok := w[n.StopReason]; ok {
			return fmt.Sprintf(w[n.Kind], n.Session), reason
		}
	case "budget":
		if b := n.Budget; b != nil && (b.Level == engine.BudgetWarning || b.Level == engine.BudgetReached) {
			period := w["day"]
			if b.Period == config.BudgetMonth {
				period = w["month"]
			}
			name := fmt.Sprintf(period, dollars(b.Amount, lang))
			if b.ProfileID != "" {
				name = fmt.Sprintf(w["profile"], name, b.ProfileID)
			}
			if b.Level == engine.BudgetReached {
				return w["reached"], fmt.Sprintf(w["spentReached"], name, dollars(b.Spent, lang))
			}
			percent := "0"
			if b.Amount > 0 {
				percent = fmt.Sprintf("%.0f", 100*b.Spent/b.Amount)
			}
			return w["warning"], fmt.Sprintf(w["spentWarning"], name, dollars(b.Spent, lang), percent)
		}
	}
	return n.Title, n.Body
}

// dollars writes an amount in US dollars as the language writes money, as the window
// does: $1,250.00, 1.250,00 $ and $ 1.250,00, with a no-break space.
func dollars(v float64, lang string) string {
	s := fmt.Sprintf("%.2f", v)
	whole, cents, _ := strings.Cut(s, ".")
	neg := strings.HasPrefix(whole, "-")
	whole = strings.TrimPrefix(whole, "-")
	group, point := ",", "."
	if lang == "de" || lang == "nl" {
		group, point = ".", ","
	}
	var b strings.Builder
	for i, r := range whole {
		if i > 0 && (len(whole)-i)%3 == 0 {
			b.WriteString(group)
		}
		b.WriteRune(r)
	}
	n := b.String() + point + cents
	if neg {
		n = "-" + n
	}
	switch lang {
	case "de":
		return n + " $"
	case "nl":
		return "$ " + n
	}
	return "$" + n
}
