# Sitzungen und Bereiche

In einer Sitzung arbeiten, viele davon in Ordnung halten und mehrere nebeneinander sehen.

## Eine Sitzung starten

In der Seitenleiste **Neue Sitzung** wählen, oder ⌘K drücken und sie dort auswählen. Im Nachrichtenfeld:

- Enter sendet, Shift+Enter fügt eine Zeile hinzu.
- `@` hängt eine Datei aus dem Projekt an.
- `/` listet Befehle auf: die Modi und was der Agent anbietet.
- Dateien hineinziehen oder einfügen, um sie anzuhängen.

Während eine Runde läuft, wartet eine neue Nachricht in einer Warteschlange, bis die Runde endet. **Stoppen** beendet die Runde.

## Freigaben beantworten

Will der Agent etwas tun, worüber die Richtlinie nicht entscheidet, erscheint eine Karte mit dem Befehl oder den Dateien. **Einmal erlauben**, **Immer erlauben** oder **Ablehnen** wählen, auf Wunsch mit einer Notiz für den Agenten.

> **Immer erlauben** speichert eine Regel. Das Menü daneben legt fest, ob sie für diese Sitzung oder überall gilt.

## Zurückgehen oder abzweigen

Auf eine der eigenen Nachrichten zeigen:

- **Bis hier zurückspulen** setzt die Dateien und das Gespräch auf den Stand vor dieser Nachricht zurück und legt die Nachricht wieder ins Feld. Vorher kommt eine Rückfrage.
- **Hier abzweigen** startet eine neue Sitzung mit dem Gespräch bis zu dieser Nachricht.

Ist der Kontext zu 80 % voll, fasst **Kontext verdichten** im Kopf das Gespräch zusammen, damit jede Nachricht weniger sendet.

## Sitzungen mit Worktrees trennen

**In einem eigenen Git-Worktree und Branch arbeiten** im Formular für eine neue Sitzung gibt der Sitzung einen eigenen Checkout, damit Sitzungen im selben Projekt nicht dieselben Dateien bearbeiten. Dafür muss das Projekt vertrauenswürdig sein.

Um die Arbeit zurückzuholen, sie committen und dann im Tab „Diff“ **Zusammenführen…** wählen. Nur committete Arbeit wird zusammengeführt.

## Ordnen

- **Umbenennen, taggen, archivieren, löschen:** im Menü ⋯ der Sitzung oder per Rechtsklick auf sie in der Seitenleiste.
- **Suchen:** Die Filter-Schaltfläche neben **Sitzung suchen** gruppiert und filtert nach Status, Tag und archiviert. **Filter speichern…** behält einen Filter.

> Löschen entfernt das Transkript von der Festplatte. Das lässt sich nicht rückgängig machen.

## Mehrere gleichzeitig

| Ansicht | Wofür |
| --- | --- |
| Bereiche | Die Schaltflächen unten in der Seitenleiste zeigen eine bis vier Sitzungen. Eine Sitzung hineinziehen oder mit ⌘-Klick anklicken. |
| Board | Eine Karte je Issue aus dem Backlog eines Arbeitsablaufs, eine Zeile je Projekt: Backlog, In Arbeit, In Prüfung, Erledigt. |
| Warteschlange | Aufgaben, die als Sitzungen starten, sobald Platz ist. Ihr Symbol in der Seitenleiste zeigt, wie viele warten. |
| Braucht Aufmerksamkeit (⌘0) | Alles, was auf eine Antwort wartet, über alle Projekte hinweg. J und K wechseln zwischen den Anfragen. |

## Der rechte Seitenbereich

**Seitenbereich** im Kopf der Sitzung blendet ihn ein.

| Tab | Zeigt |
| --- | --- |
| Freigaben | Was auf eine Antwort wartet, die Regeln der Sitzung, was entschieden wurde |
| Diff | Änderungen seit dem Start der Sitzung; stagen, committen, zusammenführen |
| Dateien | Das Projekt, mit den Änderungen des Agenten markiert |
| Terminal | Eine Shell in der Umgebung der Sitzung |
| Vorschau | Die laufende App, nach Port |
| Info | Modell, Modus, Token, Kosten und was das Modell sieht |

> Das Terminal läuft mit den Rechten des eigenen Benutzerkontos, ohne Freigaben, und der Agent sieht nicht, was dort getippt wird.
