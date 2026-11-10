# Arbeitsabläufe

In Stufen arbeiten: den Entwurf klären, aufschreiben, in Scheiben schneiden und die unbeaufsichtigten Teile Agenten überlassen.

## Was ein Arbeitsablauf ist

Ein Arbeitsablauf gibt einer Sitzung Slash-Befehle, einen je Stufe. Der eingebaute, **Plan, slice, run AFK**, folgt der Arbeitsweise mit Agenten von Matt Pocock:

| Befehl | Was passiert |
| --- | --- |
| `/grill <brief>` | Der Agent stellt Fragen zum Briefing, eine nach der anderen, jede mit der Antwort, die er vorschlägt. Im Modus „Planen“, also wird nichts geschrieben. |
| `/prd` | Er schreibt den geklärten Entwurf als PRD nach `backlog/prds/`. |
| `/slice` | Er teilt das PRD in Issues in `backlog/issues/` auf: dünne Scheiben durch alle Schichten, jede als AFK oder Mensch markiert, mit dem, was sie blockiert. Dann committet er sie. |
| `/afk` | Öffnet den Dialog für den AFK-Lauf. |
| `/review` | Ein zweiter Agent prüft die Änderungen dieser Sitzung, nur lesend. |
| `/cleanup` | Räumt erledigte Issues und PRDs aus dem Projekt. |

Jeder Werkzeugaufruf geht weiterhin durch die Richtlinie, in jeder Stufe.

## Einen starten

1. **Einstellungen → Arbeitsabläufe** öffnen und auf **Fähigkeiten installieren** klicken. Die Fähigkeiten der Stufen kommen in den eigenen Fähigkeitenspeicher von Freeloop, mit vermerkter Herkunft. Nach `~/.claude` oder ins Projekt wird nichts geschrieben.
2. In einer Sitzung den Arbeitsablauf in der Liste neben dem Optimierer wählen, oder für eine neue Sitzung in der Palette (⌘K) **Arbeitsablauf starten…** wählen.
3. `/` tippen, um die Stufen zu sehen.

**Für neue Sitzungen** auf der Seite Arbeitsabläufe legt eine Vorgabe fest, und ein Profil kann eine eigene wählen. **Aus** gewinnt auf beiden Seiten.

## Das Backlog

Issues sind Markdown-Dateien, die die Agenten schreiben und die App liest, nur in einem vertrauenswürdigen Projekt:

```
---
title: Search drafts
status: open        # open or done
type: afk           # afk: an agent may do it alone; human: it needs you
blockedBy: [001-store-a-draft]
---
```

Das **Board** zeigt eine Zeile für jedes vertrauenswürdige Projekt mit Backlog, mit einer Karte für jedes Issue in vier Spalten: **Backlog**, **In Arbeit**, **In Prüfung** und **Erledigt**. Eine Karte zeigt Titel, Nummer, ob ein Agent (AFK) oder ein Mensch sie erledigt, und im Backlog, worauf sie wartet. Ein Issue, das die App nicht lesen kann, zählt als eines für einen Menschen. Ein Projekt mit Backlog-Ordner, dem noch nicht vertraut wird, zeigt eine Zeile mit der Schaltfläche **Projekt vertrauen**; aus seinem Ordner wird nichts gelesen, bis ihm vertraut wird. Gibt es nirgends ein Backlog, sagt das Board das und bietet **Workflow starten…** an. Ein Klick auf eine Karte öffnet ihren Bereich: den Text des Issues, seine Datei, worauf es wartet und was es aufhält, und die Sitzungen, die daran gearbeitet haben; jede davon lässt sich öffnen. Der Bereich zeigt nur; Escape schließt ihn. Das Board zeigt nur, wo jedes Issue steht, nach seiner Datei und dem Lauf; Karten werden nicht von Hand verschoben. Gewöhnliche Sitzungen sind nicht darauf: Sie stehen in der Seitenleiste.

Braucht ein Agent, der an einem Issue arbeitet, eine Antwort, bleibt seine Karte in **In Arbeit** mit einer deutlichen Markierung **Wartet auf eine Entscheidung**, und die Anfrage, eine Freigabe oder eine Frage, steht auf der Karte: Sie lässt sich dort beantworten, oder mit **Zugehörige Sitzung öffnen**. Der Kopf des Boards zählt solche Issues, und ein Klick darauf führt zum ersten.

In **In Prüfung** steht ein Issue auf **Agent prüft**, solange der zweite Agent es liest, und auf **Bereit für die QA**, wenn die eigene Prüfung dran ist: Die Karte zeigt den Anfang der Antwort aus dem Review, und **QA erledigt** verschiebt sie nach **Erledigt**. Hält ein Lauf bei einem Issue an, geht es mit einem roten Hinweis auf den Grund zurück ins **Backlog**, bis der nächste Lauf startet.

**Erledigt** zeigt die letzten fünf Issues eines Projekts, die zuletzt von einem Lauf abgeschlossenen zuerst; **Alle … zeigen** zeigt den Rest. Der Pfeil neben dem Projektnamen klappt seine Zeile ein, und sie bleibt eingeklappt, nur eine Zeile mit einem wartenden Issue öffnet sich von selbst. Bei mehreren Projekten zeigt die Liste im Kopf eines davon oder alle.

**Erledigte archivieren** entfernt erledigte Issues und PRDs, deren Issues alle erledigt sind, in einem eigenen Commit, damit spätere Agenten nicht von veralteten Dokumenten in die Irre geführt werden. Die Git-Historie behält sie: `git log -- backlog/` listet sie auf, und `git show` holt eines zurück.

## Die AFK-Issues ausführen

**AFK-Issues ausführen…** auf dem Board, `/afk` oder **AFK-Issues ausführen: <Projekt>** in der Palette öffnet den Dialog für den Lauf. Er zeigt das erste Issue, die Profile für die Issues und ihre Reviews und die Befehle, die der Lauf brauchen wird: seine eigenen Git-Befehle und die Tests und Prüfungen des Projekts.

- **Für die Sitzungen dieses Laufs erlauben** lässt diese Befehle ohne Rückfrage laufen, nur in den Sitzungen des Laufs. Alles andere fragt weiterhin und wartet in **Braucht Aufmerksamkeit** auf eine Antwort.
- Der Lauf bearbeitet ein Issue nach dem anderen, jedes in einer neuen Sitzung, in einem eigenen Git-Worktree und Branch. Der eigene Checkout bleibt, wie er ist.
- Ein Issue ist erledigt, wenn seine Datei `status: done` sagt. Dann committet der Lauf, was übrig ist, und ein zweiter Agent prüft es, bevor das nächste Issue beginnt.
- Er hält an, wenn kein AFK-Issue bereit ist, bei seiner Obergrenze, wenn ein Budget erreicht ist oder wenn eine Sitzung endet, ohne ihr Issue als erledigt zu markieren. **Pausieren** und **Stoppen** sind auf dem Board.

> Das Backlog muss zuerst committet sein: Der Lauf startet vom letzten Commit.

Ein Zeitplan kann einen Lauf über Nacht starten: Unter **Einstellungen → Zeitpläne** bei **Startet** die Option **Einen AFK-Lauf des Backlogs** wählen.

## Das Ergebnis per QA prüfen

Jedes fertige Issue wartet in **Braucht Aufmerksamkeit** als **Bereit für QA**, mit der Antwort aus dem Review. Die Änderung ausprobieren, dann auf **QA erledigt** klicken. Bestehen die Issues des Laufs, seinen Branch im Tab „Diff“ einer seiner Sitzungen zusammenführen.

## Eigene hinzufügen

Eine Datei in den Ordner `workflows` des Datenordners legen. Sie läuft erst, nachdem sie auf der Seite Arbeitsabläufe mit **Freigeben** freigegeben ist, und jede Änderung an der Datei braucht eine neue Freigabe. Ein Arbeitsablauf kann das Fragen nie abschalten.
