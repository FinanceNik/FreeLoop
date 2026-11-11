# Tastatur und Befehlspalette

Mit der Palette alles finden und die Tastaturkurzbefehle ansehen oder ändern.

## Die Befehlspalette nutzen

⌘K öffnet die Palette; ein zweites Mal gedrückt oder mit Escape schließt sie sich. Sie erreicht fast alles, was sich anklicken lässt: Seiten, Sitzungen, Aktionen und die Bedienelemente der offenen Sitzung.

- Mit ↑ und ↓ bewegen und mit Enter den hervorgehobenen Eintrag ausführen.
- Hervorgehoben ist erst etwas nach dem Tippen oder Bewegen, sodass ⌘K und dann ein reflexhaftes Enter nichts ausführt.
- Ein Hinweis rechts zeigt den Tastaturkurzbefehl, der dasselbe tut.

Vor dem Tippen zeigt sie vier Abschnitte:

| Abschnitt | Zeigt |
| --- | --- |
| Wartet auf Antwort | Freigaben, die eine Antwort brauchen |
| Aktionen | Zum Beispiel Neue Sitzung und Demo ausprobieren |
| Letzte Sitzungen | Die fünf neuesten |
| Wechseln zu | Seiten wie Board, Warteschlange, Anbieter, Profile, Richtlinie und Einstellungen |

Tippen sucht. Der Abschnitt Diese Sitzung enthält, was die offene Sitzung anbietet, etwa Runde stoppen, Freigaben zeigen und Fortfahren mit einem anderen Profil. Der Abschnitt Hilfe findet die Seiten dieser Hilfe.

Mit `/` am Anfang wird nur in den Befehlen der offenen Sitzung gesucht: Modi, die Befehle, die ihr Agent anbietet, die Slash-Befehle des Profils und die Prompts der MCP-Server.

## Standardkurzbefehle

{{|Unter Windows und Linux tritt Ctrl an die Stelle der Befehlstaste (Command) des Mac und Alt an die Stelle der Wahltaste (Option).}}

| Aktion | Tasten |
| --- | --- |
| Befehlspalette öffnen | ⌘K |
| Einstellungen öffnen | ⌘, |
| „Braucht Aufmerksamkeit“ öffnen | ⌘0 |
| Größerer Text | ⌘= |
| Kleinerer Text | ⌘- |
| Zum Bereich links, rechts, darüber, darunter | {{⌃⇧← ⌃⇧→ ⌃⇧↑ ⌃⇧↓|Alt+Shift+← Alt+Shift+→ Alt+Shift+↑ Alt+Shift+↓}} |
| Freigabe: einmal erlauben | Y |
| Freigabe: immer erlauben | A |
| Freigabe: ablehnen | N |

Einige Tastaturkurzbefehle sind fest:

- ⌘1 bis ⌘9 öffnen die Sitzungen, die in der Seitenleiste zuoberst stehen.
- {{⌥⌘1 bis ⌥⌘4|Alt+1 bis Alt+4}} machen einen Bereich aktiv, und {{⌥⌘← und ⌥⌘→|Alt+← und Alt+→}} wechseln zwischen Bereichen.
- Im Terminal einer Sitzung {{dienen ⌘C und ⌘V zum Kopieren und Einfügen, und ⌃C geht an die Shell.|dienen Ctrl+Shift+C und Ctrl+Shift+V zum Kopieren und Einfügen, ebenso Ctrl+V und, bei markiertem Text, Ctrl+C. Sonst geht Ctrl+C an die Shell.|dienen Ctrl+Shift+C und Ctrl+Shift+V zum Kopieren und Einfügen, und Ctrl+C geht an die Shell.}}

## Einen Tastaturkurzbefehl ändern

**Einstellungen → Tastatur** öffnen, die Taste neben einer Aktion wählen und die neue Kombination drücken. Escape lässt sie, wie sie war, **Zurücksetzen** stellt den Standard wieder her.

Freeloop lehnt eine Taste ab, die nicht funktionieren kann, und sagt, warum:

- Ein App-Kurzbefehl braucht {{⌘ oder ⌃|Ctrl}} oder {{⌥|Alt}} mit einem Pfeil oder einer anderen benannten Taste, damit er auch beim Tippen funktioniert.
- Tasten, die {{macOS|Windows|Linux}} und die App für sich behalten, etwa ⌘C, ⌘V, {{⌘Q|⌘Y}} und ⌘1 bis ⌘9, sind nicht verfügbar.
- Eine Taste, die schon für eine andere Aktion belegt ist, wird abgelehnt.
- Ein Freigabe-Kurzbefehl ist ein einzelner Buchstabe ohne Zusatztaste.

## Freigaben mit Tasten beantworten

Die Buchstaben Y, A und N funktionieren nur, wenn kein Textfeld den Fokus hat. Diese funktionieren überall, auch im Nachrichtenfeld:

| Antwort | Tasten |
| --- | --- |
| Einmal erlauben | ⌘↩ |
| Immer erlauben | ⌘⇧↩ |
| Ablehnen | ⌘⇧⌫ |

In Braucht Aufmerksamkeit wählen J und K oder die Pfeiltasten, welche Anfrage die Tasten beantworten.

> Eine neue Karte ignoriert diese Tasten kurz nach dem Erscheinen, sodass eine für etwas anderes gedrückte Taste keinen Aufruf freigeben kann, der noch nicht gelesen wurde.

## Frühere Prompts und Vervollständigung im Nachrichtenfeld

Das Nachrichtenfeld vervollständigt aus dem, was früher getippt wurde. Dafür geht nichts an ein Modell.

| Tasten | Bewirkt |
| --- | --- |
| → oder Ende | Den grauen Rest eines früheren Prompts übernehmen, der so anfängt wie der Text |
| {{⌥→|Ctrl+→}} | Sein nächstes Wort übernehmen |
| Escape | Den grauen Text ausblenden, bis weitergetippt wird |
| ↑ in einem leeren Feld | Die früheren Prompts dieses Projekts zurückholen, die neuesten zuerst |
| ↑ nach einer Zeile Text | Nur die zurückholen, die mit ihr anfangen |
| ↓ | Wieder vorwärts; nach dem neuesten kommt zurück, was im Feld stand |

- Der graue Text erscheint nur bei einer einzelnen Zeile mit dem Cursor am Ende, und nur aus den Prompts dieses Projekts.
- In einem Prompt mit mehreren Zeilen bewegen ↑ und ↓ zuerst zwischen dessen Zeilen.
- **Frühere Prompts durchsuchen** in der Palette listet die Prompts, die jedes Wort im Feld enthalten, die dieses Projekts zuerst. Enter übernimmt einen ins Feld, ohne ihn zu senden.
- Nach `@` stehen die Dateien zuerst, die die Sitzung gelesen, bearbeitet oder erwähnt hat, dann die, die git als geändert meldet.
- Nach dem Namen eines Befehls und einem Leerzeichen zeigt das Feld, was der Befehl erwartet, wenn sein Agent es angibt.
