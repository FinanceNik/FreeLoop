# Fähigkeiten und MCP-Server

Fähigkeiten geben Agenten zusätzliches Wissen, MCP-Server zusätzliche Werkzeuge.

## Was eine Fähigkeit ist

Eine Fähigkeit ist ein Ordner mit einer Datei `SKILL.md`, die einem Agenten erklärt, wie er eine bestimmte Art von Aufgabe erledigt, dazu beliebige Skripte und Referenzdateien. Sie nutzt das offene Format Agent Skills, sodass für Claude Code geschriebene Fähigkeiten ohne Änderung laden.

Der Agent sieht von jeder eingeschalteten Fähigkeit nur Name und Beschreibung und lädt den vollen Text, wenn eine Aufgabe passt.

## Fähigkeiten installieren und verwalten

**Einstellungen → Fähigkeiten** listet jede Fähigkeit auf, die Freeloop gefunden hat. Sie liegen im Fähigkeitenspeicher, dem Ordner `skills` im Datenordner von Freeloop, oder in `.freeloop/skills`, `.agents/skills` oder `.claude/skills` eines vertrauenswürdigen Projekts. Freeloop ändert Projektordner nie, und eine Projektfähigkeit verdeckt eine gleichnamige Fähigkeit aus dem Speicher.

Beim Installieren wird eine Fähigkeit in den Speicher kopiert; die Quelle bleibt unverändert:

- **Aus einem Ordner installieren…** oder einen Fähigkeitsordner oder eine `.zip` auf die Ablagefläche ziehen.
- Einen Ordnerpfad, einen `.zip`-Pfad oder eine Git-URL einfügen und **Vorschau** wählen. Mit `#branch`, `#tag` oder `#commit` lässt sich eine Git-URL festlegen.
- **Aus Claude Code importieren** liest `~/.claude/skills`. Dorthin wird nichts zurückgeschrieben.

Jeder Weg zeigt zuerst eine Vorschau, in der jede Fähigkeit als neu, aktuell, geändert oder nicht ladbar markiert ist. Die gewünschten ankreuzen und die Schaltfläche zum Installieren wählen.

Jede Fähigkeit hat **Überall an** und, wenn ein Projekt gewählt ist, **In diesem Projekt**. Eine ausgeschaltete Fähigkeit wird dem Modell nicht genannt. Ein Profil kann eine Sitzung außerdem in seinem Feld **Fähigkeiten** auf bestimmte Fähigkeiten beschränken.

Eine Fähigkeit aus Git bietet **Nach Aktualisierungen suchen**, dann **Aktualisieren**, und **Zurückrollen** stellt die vorige Version wieder her. **Neue Fähigkeit…** beginnt mit einer Vorlage. **Testen…** fragt ein Modell einmal, ob es die Fähigkeit für einen selbst geschriebenen Prompt wählen würde; das verbraucht Token.

> Die Skripte einer Fähigkeit laufen nie von selbst. Der eingebaute Agent führt sie nur über das Shell-Werkzeug aus, sodass Modus und Regeln über jedes davon entscheiden wie über jeden anderen Befehl. Auch Claude Code erhält die eingeschalteten Fähigkeiten.

## Einen MCP-Server hinzufügen

MCP-Server geben einem Agenten zusätzliche Werkzeuge. Sie laufen als Befehl auf dem eigenen Rechner oder werden über HTTP erreicht. In **Einstellungen → MCP-Server** auf **Server hinzufügen** klicken und das Formular ausfüllen.

| Feld | Bedeutung |
| --- | --- |
| ID | Benennt den Server. Seine Werkzeuge erscheinen als `mcp__ID__tool`. |
| Verbindungsart | Befehl (stdio) oder Entfernt (Streamable HTTP). |
| Befehl, Argumente, Variablen | Für einen Befehlsserver. Argumente stehen je eines pro Zeile. |
| URL, Header | Für einen entfernten Server. |
| Geheimnisse | Liegen in {{dem macOS-Schlüsselbund|der Windows-Anmeldeinformationsverwaltung|dem Schlüsselbund des Systems}} und werden als Variablen übergeben oder als Header gesendet. |
| Seine Werkzeuge in jeder Sitzung anbieten | Schaltet den Server für alle ein. Ein Profil kann ausgeschaltete Server hinzufügen. |

Am schnellsten geht es mit **Vorschläge** weiter unten auf derselben Seite: GitHub, Linear, Sentry, Notion, Context7 und Playwright. **Hinzufügen…** füllt das Formular aus; gespeichert und freigegeben wird wie bei jedem anderen Server. Jede Karte sagt, wo der Server läuft und was er erreichen kann.

**Ein Verzeichnis durchsuchen…** füllt das Formular aus dem offiziellen MCP-Verzeichnis oder einer eigenen Adresse aus. **Aus Claude Code importieren…** kopiert Server aus den Einstellungen von Claude Code.

## Freigeben und anmelden

Ein neuer Server zeigt **Freigabe nötig** und genau den Befehl oder die URL, die er ausführen würde. **Freigeben und starten** wählen. Eine spätere Änderung daran, was er ausführt oder wohin er sich verbindet, auch an Variablen, Headern oder Namen von Geheimnissen, fragt erneut.

Ein entfernter Server, der OAuth verlangt, zeigt **Anmeldung nötig**. **Anmelden…** wählen und die Anmeldung im Browser abschließen. Die Token bleiben in {{dem macOS-Schlüsselbund|der Windows-Anmeldeinformationsverwaltung|dem Schlüsselbund des Systems}}, und **Abmelden** entfernt sie.

> Server laufen mit den Rechten des angemeldeten Benutzers. Befehle, die Pakete herunterladen, etwa `npx`, tragen eine Warnung; wo möglich, eine Version festlegen.

## Server in einer Sitzung nutzen

Werkzeuge erreichen den eingebauten Agenten als `mcp__server__tool`. Jeder Aufruf fragt nach, sofern keine Regel ihn erlaubt, etwa `mcp__github__*`, oder die Sitzung im Modus Umgehen läuft. Der Modus Planen sperrt sie.

- **MCP-Ressource…** neben dem Nachrichtenfeld hängt eine Ressource eines Servers an die nächste Nachricht an.
- Die Prompts eines Servers erscheinen als Slash-Befehle mit der Markierung **MCP-Prompt**.
- Externe Agenten erhalten die freigegebenen Server, die eingeschaltet sind, dazu die im Feld **MCP-Server** des Profils. Entfernte Server gehen nur an Agenten, die HTTP unterstützen.
