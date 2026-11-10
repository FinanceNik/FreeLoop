package main

import (
	"strings"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/engine"
)

// GUI-048: a notice of each kind gets its title and body in English, German and
// Dutch, from what it says happened; what the agent or the engine wrote stays as given.
func TestNoticesInTheInterfaceLanguage(t *testing.T) {
	approval := engine.Notice{Kind: "approval", Title: "Approval needed: Fix login", Body: "git push", Session: "Fix login", Detail: "git push"}
	failed := engine.Notice{Kind: "error", Title: "Session failed: Fix login", Body: "provider: 529 overloaded", Session: "Fix login", Detail: "provider: 529 overloaded"}
	finished := engine.Notice{Kind: "finished", Title: "Session finished: Fix login", Body: "Stopped at the turn limit", Session: "Fix login", StopReason: "max_turns"}
	warning := engine.Notice{Kind: "budget", Title: "Budget nearly spent", Body: "…", Budget: &engine.BudgetState{
		Budget: config.Budget{Period: config.BudgetDay, Amount: 1250}, Spent: 1100, Level: engine.BudgetWarning}}
	reached := engine.Notice{Kind: "budget", Title: "Budget reached", Body: "…", Budget: &engine.BudgetState{
		Budget: config.Budget{Period: config.BudgetMonth, Amount: 20, ProfileID: "small"}, Spent: 20.5, Level: engine.BudgetReached}}
	for _, c := range []struct {
		n           engine.Notice
		lang        string
		title, body string
	}{
		{approval, "en", "Approval needed: Fix login", "git push"},
		{approval, "de", "Freigabe nötig: Fix login", "git push"},
		{approval, "nl", "Goedkeuring nodig: Fix login", "git push"},
		{failed, "de", "Sitzung fehlgeschlagen: Fix login", "provider: 529 overloaded"},
		{failed, "nl", "Sessie mislukt: Fix login", "provider: 529 overloaded"},
		{finished, "en", "Session finished: Fix login", "Stopped at the turn limit"},
		{finished, "de", "Sitzung beendet: Fix login", "Angehalten: Rundenlimit erreicht"},
		{finished, "nl", "Sessie klaar: Fix login", "Gestopt: beurtlimiet bereikt"},
		{warning, "en", "Budget nearly spent", "The daily budget of $1,250.00: $1,100.00 spent so far, 88%."},
		{warning, "de", "Kostenrahmen fast ausgeschöpft", "Der tägliche Kostenrahmen von 1.250,00 $: bisher 1.100,00 $ ausgegeben, 88 %."},
		{warning, "nl", "Budget bijna op", "Het dagbudget van $ 1.250,00: tot nu toe $ 1.100,00 uitgegeven, 88%."},
		{reached, "en", "Budget reached", "The monthly budget of $20.00 for profile small: $20.50 spent so far. Running sessions go on; the queue waits."},
		{reached, "de", "Kostenrahmen erreicht", "Der monatliche Kostenrahmen von 20,00 $ für das Profil small: bisher 20,50 $ ausgegeben. Laufende Sitzungen laufen weiter; die Warteschlange wartet."},
		{reached, "nl", "Budget bereikt", "Het maandbudget van $ 20,00 voor profiel small: tot nu toe $ 20,50 uitgegeven. Lopende sessies gaan door; de wachtrij wacht."},
	} {
		title, body := noticeText(c.n, c.lang)
		if title != c.title || body != c.body {
			t.Errorf("%s %s: got %q / %q, want %q / %q", c.n.Kind, c.lang, title, body, c.title, c.body)
		}
	}
}

// An unknown kind, stop reason or language shows the engine's English rather than
// nothing.
func TestAnUnknownNoticeKeepsTheEnginesText(t *testing.T) {
	for _, n := range []engine.Notice{
		{Kind: "something-new", Title: "Something new", Body: "It happened"},
		{Kind: "finished", Title: "Session finished: x", Body: "Odd", Session: "x", StopReason: "odd"},
		{Kind: "budget", Title: "Budget reached", Body: "Spent"},
	} {
		for _, lang := range []string{"de", "nl", "fr"} {
			if title, body := noticeText(n, lang); title != n.Title || body != n.Body {
				t.Errorf("%+v in %s: %q / %q", n, lang, title, body)
			}
		}
	}
}

// GUI-048: the menu's own titles follow the language, built again on a change
// without a restart, and the Help menu is the app's own.
func TestTheMenuFollowsTheLanguage(t *testing.T) {
	app := application.New(application.Options{Name: "Freeloop"})
	d := &desktop{app: app}
	titles := func(m *application.Menu) []string {
		var out []string
		for i := 0; m.ItemAt(i) != nil; i++ {
			out = append(out, m.ItemAt(i).Label())
		}
		return out
	}
	want := map[string][]string{
		"en": {"File", "Edit", "View", "Window", "Help"},
		"de": {"Datei", "Bearbeiten", "Darstellung", "Fenster", "Hilfe"},
		"nl": {"Bestand", "Wijzig", "Weergave", "Venster", "Help"},
	}
	for _, lang := range []string{"de", "nl", "en"} {
		d.setLanguage(lang)
		if d.language() != lang {
			t.Fatalf("language %q, want %q", d.language(), lang)
		}
		menu := appMenu(d.language(), func() {})
		got := strings.Join(titles(menu), ",")
		for _, title := range want[lang] {
			// A Mac's own names (Ablage, Archief) are its platform's.
			if title == "Datei" || title == "Bestand" {
				if w := thisPlatform.MenuWords[lang]["File"]; w != "" {
					title = w
				}
			}
			if !strings.Contains(got, title) {
				t.Errorf("%s: menu %s has no %s", lang, got, title)
			}
		}
		if help := menu.FindByLabel(map[string]string{"en": "Freeloop Help", "de": "Freeloop-Hilfe", "nl": "Freeloop Help"}[lang]); help == nil {
			t.Errorf("%s: no Help item of the app's own: %s", lang, got)
		}
		if menu.FindByLabel("Learn More") != nil {
			t.Errorf("%s: Wails' Help item is still there", lang)
		}
	}
	d.setLanguage("fr")
	if d.language() != "en" {
		t.Errorf("an unknown language gives %q", d.language())
	}
}
