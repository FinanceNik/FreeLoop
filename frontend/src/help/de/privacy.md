# Datenschutz und Daten

Was Freeloop auf dem Computer behält, was es an Modellanbieter sendet und womit es sich sonst verbinden kann.

## Was auf dem Computer bleibt

Freeloop legt seine Daten in einem Ordner ab, {{`~/Library/Application Support/freeloop/`|`%AppData%\freeloop\`|`~/.config/freeloop/` (oder unter `$XDG_CONFIG_HOME`)}}, den nur das eigene Benutzerkonto öffnen kann. Alles darin ist lesbar und bleibt auf diesem Computer. Es gibt keine Telemetrie, keine Analyse und keine Absturzberichte.

- `sessions/<id>/events.jsonl`: das Transkript jeder Sitzung
- `config.yaml`, `providers.yaml`, `rules.yaml`, `mcp.yaml` und die Ordner `profiles`, `environments`, `presets`, `optimizers` und `skills`: Einstellungen und Fähigkeiten
- `freeloop.db`: der Index der Sitzungen und Projekte
- `logs/freeloop.log`: das Protokoll der App

API-Keys, MCP-Anmelde-Token und Geheimnisse der Umgebungen liegen {{im macOS-Schlüsselbund|in der Windows-Anmeldeinformationsverwaltung|im Schlüsselbund des Systems}}. Dateien wie `providers.yaml` enthalten nur Verweise darauf, und Schlüssel der Anbieter landen nie in der Umgebung eines Befehls.

Die aus dem Nachrichtenfeld gesendeten Prompts werden zum Vervollständigen und Zurückholen aufbewahrt (Tastatur und Befehlspalette), im eigenen Speicher des App-Fensters statt in diesem Ordner: bis zu 500, ihm bekannte Geheimnisse vorher ersetzt. Ein Prompt verschwindet, wenn seine Sitzung gelöscht wird. Die Liste beginnt leer mit der Version, die sie eingeführt hat, und im Servermodus führt jeder Browser seine eigene.

> Transkripte sind reiner Text, nur durch Dateiberechtigungen geschützt. Freeloop ersetzt ihm bekannte Geheimnisse, bevor es ein Transkript speichert. Nicht erkennen kann es ein in eine Nachricht getipptes Passwort, einen Schlüssel, den ein Befehl von anderswo ausgibt, oder ein Geheimnis mit weniger als acht Zeichen.

## Was an einen Anbieter geht

Ruft der eingebaute Agent ein Modell auf, erhält der gewählte Anbieter die Unterhaltung, den System-Prompt, die Werkzeugdefinitionen und den Index der Fähigkeiten. Dazu gehören Dateiinhalte, die der Agent gelesen hat, Befehlsausgaben und mit `@` angehängte Dateien.

Was ein Anbieter mit den Daten macht, regeln seine eigenen Bedingungen. Ein Modell, das auf dem eigenen Computer läuft, etwa in Ollama oder LM Studio, behält sie dort. Was hinausgeht, zeigt im Tab **Info** einer Sitzung der Abschnitt **Was das Modell sieht**.

> Ein externer Agent baut seine Anfragen selbst. Freeloop sieht nur die Nachrichten und Werkzeugaufrufe, daher bleibt „Was das Modell sieht“ für solche Sitzungen leer. Der Agent spricht unter seiner eigenen Anmeldung mit seinem Hersteller, und Freeloop liest oder speichert diese Anmeldung nie.

## Weitere Verbindungen

Neben dem, was eingerichtet oder angefordert wird, etwa Anbieter, MCP-Server, Agenten, SSH-Rechner, die Git-Adresse einer Fähigkeit oder eine Seite, die der Agent mit Freigabe abruft, verbindet sich Freeloop mit fünf Dingen, und nur, wenn sie genutzt werden:

- Die Suche nach Updates ist aus, bis **Einmal täglich nach einer neueren Version suchen** unter Einstellungen → Netzwerk eingeschaltet wird. Die Suche fragt GitHub nach der neuesten Veröffentlichung und sendet nichts über diese Kopie der App. **Jetzt prüfen** führt sie einmal aus.
- Der Download eines Token-Optimierers geschieht nur beim Klick auf seine Schaltfläche zum Installieren, etwa **RTK installieren**, unter Einstellungen → Token-Optimierer. Er kommt von GitHub, in einer festgelegten Version, und wird gegen eine bekannte Prüfsumme geprüft.
- Das Sprachmodell der Spracheingabe wird nur beim Klick auf **Herunterladen** geladen, im Nachrichtenfeld oder unter Einstellungen → Spracheingabe. Es kommt von Hugging Face, unter Windows und Linux whisper.cpp von GitHub, jeweils festgelegt und gegen eine bekannte Prüfsumme geprüft. Das Gesagte wird auf diesem Computer zu Text und nirgendwohin gesendet.
- Das MCP-Verzeichnis wird nur kontaktiert, wenn es über **Ein Verzeichnis durchsuchen…** auf der Seite MCP-Server durchsucht wird. Es erhält die Suchbegriffe an der gewählten Verzeichnisadresse.
- Die Benachrichtigung für ein Telefon geht an dessen Push-Dienst (den von Apple, Google oder Mozilla, je nachdem, welchen sein Browser nutzt), und nur für ein Telefon, bei dem **Benachrichtige mich** an ist. Die Nachricht ist so verschlüsselt, dass nur dieses Telefon sie lesen kann, und enthält nur, was geschehen ist, und die ID einer Sitzung, nie ihren Inhalt. Der Push-Dienst sieht, dass eine Nachricht gesendet wurde und wann. Der Nur-lokal-Modus sendet keine.

Ist unter Einstellungen → Netzwerk eine Suchadresse für die Websuche des eingebauten Agenten eingetragen, gehen die Suchanfragen dorthin. Jede Suche fragt vorher nach, sofern keine Erlaubnisregel und nicht der Umgehungsmodus sie durchlässt.

Ist **Dem Telefon antworten** unter Einstellungen → Telefon an, lesen gekoppelte Telefone die Sitzungen über das eigene Tailscale-Netz. Freeloop selbst antwortet nur auf diesem Computer und verbindet sich dafür nur zur Benachrichtigung oben. Der Nur-lokal-Modus schaltet es aus.

## Nur-lokal-Modus

Unter Einstellungen → Netzwerk beschränkt **Nur lokal: Die App verbindet sich mit nichts außer diesem Computer** die eigenen Verbindungen der App auf diesen Computer.

- Anbieter und MCP-Server anderswo werden unerreichbar. Ollama, LM Studio und Server auf localhost funktionieren weiter.
- Jeder blockierte Versuch wird auf derselben Seite aufgeführt.
- Docker-Container, die währenddessen erstellt werden, haben kein Netzwerk.

> Programme, die die App startet, etwa Befehle, externe Agenten und stdio-MCP-Server, sind nicht abgedeckt. Diese Programme laufen mit dem eigenen Benutzerkonto. Für sie eignet sich eine Docker-Umgebung.

## Sicherungen, Exporte und Berichte

- **Konfiguration sichern…** unter Einstellungen → Daten schreibt Einstellungen, Anbieter, Profile, Umgebungen, Regeln, MCP-Server, Token-Optimierer und Agent-Vorlagen in eine Datei. Geheimnisse, Transkripte und Fähigkeiten sind nicht enthalten.
- **Aus einer Sicherung wiederherstellen…** zeigt zuerst, was es tun würde. Alles, was Befehle ausführen oder die Richtlinie lockern könnte, wartet auf Freigabe.
- **Als Markdown exportieren…** und **Als JSON exportieren…** im Menü einer Sitzung schreiben die Unterhaltung in eine selbst gewählte Datei, nur auf Anforderung.
- **Bericht kopieren** unter **Problem melden** in Einstellungen → Daten kopiert eine kurze Diagnose in die Zwischenablage, ohne bekannte Geheimnisse und ohne den Pfad des Benutzerordners. Nichts wird irgendwohin gesendet.

> Eine Sicherung enthält Werte, die als reiner Text in Konfigurationsdateien eingetragen wurden; daher vor dem Weitergeben prüfen. Ein Export kann alles enthalten, was der Agent gelesen hat; daher vor dem Weitersenden lesen. Was ein Bericht enthält, steht auf der Seite Fehlerbehebung.
