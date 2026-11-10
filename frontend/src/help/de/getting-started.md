# Erste Schritte

Von der frischen Installation zur ersten Sitzung.

## Die Demo ausprobieren

Auf dem ersten Bildschirm **Mit einer Demo ausprobieren** wählen, oder ⌘K drücken und **Demo ausprobieren** auswählen. Die Demo braucht keinen Schlüssel und kein Netzwerk.

- Dabei entsteht ein Wegwerfprojekt, und darin startet eine Sitzung mit einem Agenten nach festem Skript.
- Der Agent probiert Dinge, die die Richtlinie erlaubt, erfragt und verweigert, und die Freigaben werden wie in einer echten Sitzung beantwortet.
- **Demo entfernen** im Banner der Sitzung löscht die Demo-Sitzung, ihren Wegwerfordner und ihren Projekteintrag, sonst nichts.

## Der Einrichtungshilfe folgen

**Willkommen bei Freeloop** führt beim ersten Öffnen der App durch vier Schritte.

- Festlegen, wie Agenten laufen: einen Agenten mit bestehender Anmeldung über seine Schaltfläche wählen, etwa **Claude Code nutzen**, oder **Einen API-Schlüssel oder ein lokales Modell nutzen** öffnen und einen Anbieter hinzufügen.
- Aus Claude Code importieren (optional): übernimmt dessen MCP-Server, Berechtigungsregeln und Fähigkeiten.
- Ein Projekt wählen, mit **Auswählen…**. Das Kästchen **Diesem Projekt vertrauen** ist anfangs aus.
- Eine Sitzung starten: Das Formular öffnet sich mit dem gewählten Ordner.

**Einrichtungshilfe überspringen** beendet sie. Um sie erneut zu sehen, ⌘K drücken und **Einrichtungshilfe** wählen.

## Einen Anbieter hinzufügen

Ein Anbieter ist ein Modelldienst, mit dem der eingebaute Agent spricht. Für einen Agenten mit bestehender Anmeldung ist keiner nötig.

- **Einstellungen → Anbieter** öffnen und **Anbieter hinzufügen** wählen.
- Die Art wählen, Name und Basis-URL prüfen und den API-Schlüssel einfügen. Lokale Server brauchen keinen.
- Optional ein Standardmodell festlegen, dann **Speichern** wählen.

Beim Speichern wird die Verbindung getestet, und die Modelle werden aufgelistet. Schlüssel kommen in {{den macOS-Schlüsselbund|die Windows-Anmeldeinformationsverwaltung|den Schlüsselbund des Systems}}, nicht in eine Datei. Mehr dazu unter „Anbieter und Modelle“.

## Die erste Sitzung starten

In der Seitenleiste **Neue Sitzung** wählen, oder ⌘K drücken und dort auswählen.

- Das Projekt wählen, oder mit Auswählen… einen Ordner bestimmen.
- Profil auf dem Standard lassen. Für den eingebauten Agenten einen Anbieter und ein Modell wählen.
- Berechtigungsmodus auf dem Standard lassen. Sofern Profil oder Umgebung nichts anderes festlegen, ist das Fragen: Der Agent fragt also, bevor er Dateien ändert oder Befehle ausführt.
- **Sitzung starten** wählen und dann im Nachrichtenfeld eine Aufgabe beschreiben.

## Einem Projekt vertrauen

Ein Projektordner gilt als nicht vertrauenswürdig, bis etwas anderes festgelegt wird, denn ein Repository kann Dateien enthalten, die einen Agenten steuern. Bis dahin nutzen Sitzungen darin nur die globalen Einstellungen. Freeloop liest weder seine `AGENTS.md` oder `CLAUDE.md` noch seine `.freeloop/`-Einstellungen, führt seine Hooks nicht aus und führt darin kein `git status` aus.

- Im Formular für neue Sitzungen listet **Diesem Ordner vertrauen?** die Hook-Befehle auf, die Vertrauen erlauben würde. **Diesem Projekt vertrauen** wählen oder ohne Vertrauen starten.
- In einer laufenden Sitzung im Banner **Projekt vertrauen** wählen.
- Um das Vertrauen zurückzunehmen, **Diesem Projekt nicht mehr vertrauen…** unter **Weitere Aktionen** (**⋯** in der Kopfzeile) oder in der Befehlspalette wählen.

Die Arbeit in einem eigenen Worktree, der Umgehungsmodus, der Start aus einem GitHub-Issue und der Vergleich von Profilen setzen Vertrauen voraus.

> Vertrauen schaltet Freigaben nicht ab: Der Agent fragt weiterhin, Verbotsregeln haben weiterhin Vorrang, und die `.freeloop/`-Dateien eines Projekts können Berechtigungen verschärfen, aber nie lockern. Vertrauen lässt jedoch die Hooks des Projekts laufen; daher nur Ordnern vertrauen, deren Inhalt bekannt ist.

## Wie es weitergeht

- Sitzungen und Bereiche: das Nachrichtenfeld und mehrere Sitzungen gleichzeitig.
- Agenten und Profile: externe Agenten und Profile.
- Berechtigungen und die Richtlinie: wie Freigaben und Regeln entscheiden.
