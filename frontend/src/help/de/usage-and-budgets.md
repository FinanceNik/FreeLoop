# Nutzung, Preise und Kostenrahmen

Sehen, was die Sitzungen verbrauchen und kosten, und Kostenrahmen festlegen, die warnen.

## Die Nutzungsansichten öffnen

Die Schaltfläche **Nutzung** in der Seitenleiste öffnet die Nutzungsseite. Auf einen Bereich gezogen, lässt sich die Nutzung neben den Sitzungen verfolgen. In einem Bereich blättern Pfeile durch Letzte Stunden, Letzte 12 Monate, Token und Nach Modell.

Sobald etwas, das heute verbraucht wurde, einen Preis hat oder ein Kostenrahmen fast erreicht ist, zeigt die Seitenleiste auch die geschätzten Kosten von heute.

## Die Diagramme lesen

| Reiter | Zeigt |
| --- | --- |
| Letzte Stunden | Die letzten 12 oder 24 Stunden, mit jedem Modellaufruf, sobald er fertig ist. |
| Token und Kosten | 7 Tage bis 12 Monate, verglichen mit dem Zeitraum davor. |
| Aktivität, Freigaben, Effizienz | Was Agenten getan haben, wer über jede Freigabe entschieden hat, was eine Runde braucht. |

Unter **Token und Kosten** teilt **Gruppieren nach** das Diagramm nach Token-Art, Modell, Anbieter oder Agent oder Projekt auf. Filter gelten für jeden Reiter, und **Filter zurücksetzen** entfernt sie. **CSV exportieren…** speichert die Daten.

## Was geschätzt bedeutet

Freeloop zählt die Token jedes Modellaufrufs, auch die externer Agenten, die sie melden. Kosten sind eine Schätzung: Token mal ein Preis in US-Dollar pro Million Token.

Die Preise kommen aus **Einstellungen → Kosten**. Modelle von Anthropic, OpenAI und Google nutzen die offiziellen Listenpreise ihrer API. Eine unter Preise hinzugefügte Zeile hat Vorrang, sodass sich ein eigener Preis eintragen lässt oder einer für ein Modell, das nicht gelistet ist. Ein Modell kann ein Muster wie `claude-*` sein, und die erste passende Zeile gilt.

- Ein Modell ohne Preis zählt als kostenlos. Die Seite sagt, wie viele Token das sind, und bietet **Preise hinzufügen** an.
- Manche Agenten melden für eine Runde keine Token oder nur eine Summe. Ein Hinweis sagt, welche, und die Kosten lassen diese Runden weg.
- Ein Agent, der Token meldet, aber nicht sein Modell, etwa Gemini CLI, lässt sich trotzdem bepreisen. Dazu in seinem Profil auf der Seite Profile **Modell, auf dem der Agent läuft** setzen.
- Eine Zahl, die etwas auslässt, endet auf **+**.

> Ist im Zeitraum nichts bepreist, bleibt die Auswahl Kosten gesperrt, bis ein Preis hinzugefügt ist.

## Limits der Agenten

Der Abschnitt Nutzungslimits zeigt, wie viel eines Kontolimits ein Agent als verbraucht gemeldet hat, wann es zurückgesetzt wird und wie alt die Meldung ist. Ein Limit kann **Nahe am Limit** oder **Limit erreicht** lauten.

Freeloop fragt keinen Hersteller nach diesen Zahlen. Sie kommen mit den eigenen Modellaufrufen eines Agenten, sodass ein Limit erst nach dem nächsten Aufruf des Agenten erscheint. API-Keys und lokale Modelle melden keine.

## Was ein Commit kostet

Der Tab **Effizienz** der Nutzungsseite zeigt Woche für Woche, was jeder Commit einer Sitzung gekostet hat: den Verbrauch der Sitzung seit ihrem vorigen Commit, ihre Subagenten eingeschlossen. Kostet dieselbe Art Arbeit mehr oder bringt ein Prozent des Limits eines Agenten weniger, hat sich ein Modell oder ein Abo geändert.

- Median und mittlere Hälfte halten einem einzelnen riesigen Commit stand. **Alle Ausgaben pro Commit** und **Ohne folgenden Commit** zählen auch die Arbeit, die zu keinem Commit führte.
- **Pro 1 % von** einem Limit ist, was ein Prozent davon brachte, zu heutigen Preisen. Verbrauch anderswo, etwa durch den Agenten in einem Terminal, geht unbemerkt vom selben Limit ab und lässt ein Prozent billiger aussehen.
- Eine Änderung wird genannt, sobald die letzten 2 Wochen und die 8 davor je genug Commits haben. Bis dahin sagt der Tab, dass es zu früh ist.

Commits aus einem Terminal, außerhalb jeder Sitzung, sind nicht enthalten. **Commits als CSV exportieren…** speichert die Liste.

## Kostenrahmen festlegen

Ein Kostenrahmen ist ein Betrag geschätzter Ausgaben. Unter **Einstellungen → Kosten → Kostenrahmen** auf **Kostenrahmen hinzufügen** klicken und festlegen:

| Feld | Bedeutung |
| --- | --- |
| Zeitraum | Pro Tag oder Pro Monat, in der Ortszeit. |
| Betrag in US-Dollar | Das Limit. |
| Für | Jede Sitzung oder ein Profil. |

Ein Kostenrahmen zählt nur bepreiste Nutzung, und seine Zeile sagt, was er auslässt. Jeder Kostenrahmen zeigt eine Anzeige auf dem Reiter Token und Kosten.

- Ab 80 % markieren die Nutzungsseite und die Seitenleiste den Kostenrahmen als fast erreicht, und bei eingeschalteten Benachrichtigungen kommt einmal pro Zeitraum eine Benachrichtigung.
- Ist der Betrag erreicht, geschieht dasselbe noch einmal, und die Warteschlange startet unter diesem Kostenrahmen keine neue Aufgabe.
- Die Warteschlange wartet auch, wenn das Limit eines Agenten nahe oder erreicht ist.

> Laufende Sitzungen werden nie angehalten. Ein Kostenrahmen warnt und hält die Warteschlange an, mehr nicht.
