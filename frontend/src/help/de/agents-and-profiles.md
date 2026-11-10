# Agenten und Profile

Festlegen, welcher Agent eine Sitzung ausführt, sich bei ihm anmelden und die Einrichtung als Profil speichern.

## Eingebaute und externe Agenten

| Agent | Was er ist |
| --- | --- |
| Eingebaut | Der Agent von Freeloop selbst. Er läuft auf einem hinzugefügten Anbieter und nutzt die Werkzeuge von Freeloop: Dateien lesen und bearbeiten, Befehle ausführen und suchen. |
| Extern | Ein eigenständiges Programm, das Freeloop startet, etwa Claude Code, Codex CLI, Copilot CLI, Gemini CLI, OpenCode oder goose. Es bringt eigenen Modellzugang mit und nutzt daher die eingerichteten Anbieter nicht. |

Freeloop gibt einem externen Agenten nie die Schlüssel der Anbieter oder eine Anmeldung beim Hersteller weiter. Kann ein Agent nicht starten, nennt das Formular für neue Sitzungen den Grund.

> Beim eingebauten Agenten entscheidet die Richtlinie über jede Dateiänderung und jeden Befehl. Ein externer Agent führt seine eigenen Werkzeuge in seinem eigenen Prozess aus, und nur das, wonach er fragt, erreicht Freeloop; für alles andere gelten die Verbotsregeln nicht. Für einen nicht vertrauenswürdigen Agenten eignet sich eine Docker-Umgebung.

## Bei einem externen Agenten anmelden

Ein Agent mit Abonnement wie Claude Code meldet sich über sein eigenes Programm an, in einem Terminal. Freeloop startet das unveränderte Programm und liest, speichert oder überträgt diese Anmeldung nie.

**Einstellungen → Anbieter** und das Formular für neue Sitzungen zeigen, was jeder Agent braucht:

| Agent | Was er braucht |
| --- | --- |
| Claude Code | Einmal `claude` ausführen und anmelden. |
| Codex CLI | `codex login` ausführen oder `CODEX_API_KEY` oder `OPENAI_API_KEY` als Geheimnis der Umgebung der Sitzung hinzufügen. |
| Copilot CLI | Ein Copilot-Abo, ein eigenes oder eines von einer Organisation. Einmal `npx @github/copilot login` ausführen. Jede Nachricht verbraucht Premium-Anfragen des Abos, und die Copilot-Richtlinie der Organisation muss Copilot CLI erlauben. In einem nicht vertrauenswürdigen Projekt startet es nicht, solange das Projekt Hooks dafür hat (`.github/hooks` oder Hooks in `.github/copilot/settings.json` oder `.claude/settings.json`), da es sie ohne Rückfrage ausführen würde. |
| OpenCode | `opencode auth login` ausführen oder seinen API-Key als Geheimnis der Umgebung hinzufügen. Ohne beides nutzt es die gehosteten kostenlosen Modelle von OpenCode, sodass Prompts und Code dorthin gehen. |
| goose | Installieren, dann `goose configure` ausführen. |
| Gemini CLI | Einen API-Key als `GEMINI_API_KEY` in den Geheimnissen der Umgebung hinzufügen. Keine Google-Anmeldung damit verwenden. |

Danach den Agenten in der Einrichtungshilfe mit seiner Schaltfläche wählen, etwa **Claude Code nutzen**; das macht ihn auch zum Standard für neue Sitzungen. Oder beim Start einer Sitzung sein Profil wählen.

## Ein Profil anlegen

Ein Profil ist eine gespeicherte Festlegung, wie eine Sitzung läuft, abgelegt als YAML-Datei im Ordner `profiles/` im Datenordner der App. Das eingebaute Profil Default ist der eingebaute Agent mit allen Werkzeugen.

Unter **Einstellungen → Profile** auf **Neues Profil** klicken oder bei einem bestehenden auf **Bearbeiten**. Festlegen lassen sich:

- Agent: der eingebaute oder ein externer.
- Anbieter, Modell und ein Ausweichmodell, für den eingebauten Agenten.
- Berechtigungsmodus, Aufwand, Token-Optimierer und Höchstzahl an Modellaufrufen pro Nachricht.
- System-Prompt, der den eigenen des eingebauten Agenten ergänzt.
- Angebotene Werkzeuge, Fähigkeiten und MCP-Server.
- Ob die `AGENTS.md` oder `CLAUDE.md` des Projekts gelesen wird.
- Slash-Befehle: ein Name und ein vorbereiteter Prompt.

Regeln und Hooks stehen im YAML. Zum Bearbeiten **YAML** wählen.

> Ein Hook kann einen Werkzeugaufruf stoppen. Freigeben kann er keinen.

## Profile wählen und wechseln

- Neue Sitzungen nutzen das Standardprofil, sofern im Formular für neue Sitzungen kein anderes gewählt wird.
- Der Standard lässt sich unter **Einstellungen → Sitzungen** bei Profil ändern.
- Um den eingebauten Standard zu ersetzen, bei Default auf Bearbeiten klicken und speichern. **Zurücksetzen** stellt den eingebauten wieder her.

Eine Sitzung behält ihren Agenten. Um mit einem anderen weiterzumachen, **Weitere Aktionen** öffnen und unter **Fortfahren mit** ein Profil wählen. Freeloop startet darauf eine neue Sitzung und teilt ihr mit, was in der alten geschah; die alte bleibt unverändert.

Erreicht eine Sitzung ihr Konto- oder Rundenlimit oder scheitert sie wiederholt, bietet ein Banner an, mit einem anderen Profil fortzufahren.

## Projektprofile

Ein vertrauenswürdiges Projekt kann in seinem Ordner `.freeloop/profiles/` Profile hinzufügen. Diese erscheinen im Formular für neue Sitzungen in einer eigenen Gruppe.

> Ein Projekt kann das Erlaubte nur verschärfen. Seine Profile nutzen den eingebauten Agenten, starten in Fragen oder Planen, fügen nur Verbotsregeln hinzu und können keine MCP-Server, keinen Token-Optimierer und kein Ausweichmodell hinzufügen.

In einem vertrauenswürdigen Projekt liest der eingebaute Agent zuerst `AGENTS.md` und, falls es keine `AGENTS.md` gibt, `CLAUDE.md`. Freeloop schreibt nie in diese Dateien, und ein Profil kann das Lesen abschalten.
