# Berechtigungen und die Richtlinie

Wie Freeloop entscheidet, was ein Agent tun darf, und wie sich das ändern lässt.

## Wie eine Anfrage geprüft wird

Jedes Lesen und Bearbeiten einer Datei, jeder Befehl und jeder Werkzeugaufruf geht an eine einzige Berechtigungsprüfung, beim eingebauten Agenten wie bei externen Agenten. Sie prüft geschützte Pfade, Verbotsregeln, den Modus und Erlaubnisregeln, in dieser Reihenfolge, und legt alles zur Entscheidung vor, was keine davon klärt.

Nichts wird freigegeben, weil die Zeit abgelaufen ist. Jede Entscheidung steht im Transkript, mit dem, der sie getroffen hat.

> Ein externer Agent wie Claude Code führt manche Werkzeuge in seinem eigenen Prozess aus. Was er ohne Rückfrage tut, erreicht Freeloop nie.

## Einen Modus wählen

Das Modusmenü neben dem Nachrichtenfeld zeigt den aktuellen Modus.

| Modus | Was er tut |
| --- | --- |
| **Fragen** | Lesen im Projekt läuft ohne Rückfrage. Änderungen und Befehle fragen nach, außer eine Regel entscheidet. |
| **Änderungen annehmen** | Änderungen im Projekt laufen ohne Rückfrage. Befehle fragen weiterhin nach, ebenso Änderungen an Dateien, nach denen andere Programme später handeln, etwa `AGENTS.md` oder `.github`. |
| **Planen** | Nur Lesen. Der Agent schlägt einen Plan zur Freigabe vor; alles andere wird bis dahin abgelehnt. |
| **Automatisch** | Der Agent gibt selbst frei. Nur manche Agenten bieten das, und Freeloop sieht nur, wonach sie noch fragen. |
| **Umgehen** | Aufrufe laufen ohne Rückfrage. Geschützte Pfade, Verbotsregeln und Hook-Befehle gelten weiterhin. |

> Neue Sitzungen starten in Fragen, Änderungen annehmen oder Planen. Automatisch oder Umgehen wird innerhalb einer Sitzung gewählt, und nur in einem vertrauenswürdigen Projekt. Umgehen setzt zuerst **Umgehungsmodus für Sitzungen wählbar machen** unter Einstellungen → Berechtigungen voraus. Ein Befehl, der eine Sandbox verlässt, fragt jedes Mal nach, auch im Umgehungsmodus.

## Regeln

Eine Regel nennt ein Werkzeug und optional einen Befehl oder Pfad und erlaubt oder verbietet ihn. Globale Regeln kommen aus Einstellungen → Richtlinie oder von **Immer: überall** und liegen in `rules.yaml`. **Immer: diese Sitzung** speichert eine Regel, die mit der Sitzung endet. Auch Umgebungen, Profile und vertrauenswürdige Projekte können Regeln mitbringen, doch die Regeln eines Projekts können nur verbieten.

- Eine Verbotsregel hat auf jeder Ebene und in jedem Modus Vorrang, auch beim Umgehen.
- Eine Erlaubnisregel hebt weder den Planmodus noch geschützte Pfade auf.
- Eine Shell-Regel, die auf `*` endet, passt auf ein Präfix und gibt nur einen einzelnen einfachen Befehl frei, nie eine Kette, Pipe oder Umleitung.
- Eine Shell-Verbotsregel passt auf die Wörter, die die Shell ausführt: `git "push"`, `git p\ush`, `noglob git push`, `eval "git push"` und `sh -c 'git push'` sind alle `git push`, und Flag-Buchstaben gelten als Menge: Eine Regel auf `rm -rf *` stoppt auch `rm -fr`, `rm -r -f`, `rm -rfv` und `rm --recursive --force`.
- Ein Befehl, der sein Programm oder seinen Unterbefehl erst bei der Ausführung zusammensetzt, etwa `$a push` oder `git "$sub"`, lässt sich nicht mit einer Verbotsregel vergleichen. Solange eine Shell-Verbotsregel gelten könnte, gibt ihn keine Erlaubnisregel frei (es wird gefragt) und der Umgehungsmodus lehnt ihn ab.

Auf einer Freigabekarte speichert **Immer erlauben** eine Regel für genau diesen Befehl oder Pfad. Kann eine Regel den Aufruf nicht abdecken, etwa bei einem Aufruf auf mehrere Dateien, wird er einmal erlaubt und nichts gespeichert. Die anderen Antworten sind **Einmal erlauben** und **Ablehnen**. Die Tastenkürzel sind ⌘↩, ⌘⇧↩ und ⌘⇧⌫.

> Eine Verbotsregel kann nicht aufhalten, was ein Agent in seinem eigenen Automatikmodus von sich aus tut, denn das erreicht Freeloop nie.

## Geschützte Pfade und geheime Dateien

Manche Orte werden nie gelesen oder beschrieben, in keinem Modus, egal was eine Regel sagt. Dazu gehören `~/.ssh`, `~/.aws`, die {{Schlüsselbunddateien|Dateien mit Anmeldeinformationen|Schlüsselbunddateien}}, Browserdaten, die Anmeldedateien anderer Werkzeuge und der Datenordner von Freeloop. Einstellungen → Richtlinie listet sie unter **Geschützte Pfade** auf.

In einem Projekt:

- Agenten schreiben nie in `.git` oder `.freeloop` und nie außerhalb des Projekts.
- Lesen außerhalb des Projekts fragt nach.
- Auch das Lesen einer Datei, deren Name auf Geheimnisse hinweist, etwa `.env`, fragt nach, außer eine Erlaubnisregel deckt es ab oder die Sitzung ist im Umgehungsmodus. Eine Suche über einen Ordner lässt solche Dateien aus und sagt, welche, außer man nennt sie im Pfad oder im Include; das fragt nach.

> Diese Prüfungen gelten für die Datei-Werkzeuge. Ein freigegebener Shell-Befehl läuft mit den Rechten des eigenen Kontos, außer die Umgebung hat eine Sandbox.

## Hook-Befehle

Ein Hook-Befehl ist ein Shell-Befehl, der an einem bestimmten Punkt einer Sitzung läuft. Unter Einstellungen → Berechtigungen, bei **Hook-Befehle**, **Hook-Befehl hinzufügen** wählen, dann **Start der Sitzung**, **Vor einem Werkzeugaufruf**, **Vor einer Rückfrage** oder **Nach einem Werkzeugaufruf** auswählen und **Speichern** drücken. Das Standard-Zeitlimit beträgt 30 Sekunden.

Der Hook-Befehl bekommt das Ereignis als JSON auf der Standardeingabe, und sein Exit-Code entscheidet:

- 0 lässt weitermachen.
- 2 blockiert, mit der Standardfehlerausgabe als Grund. Ein blockierter Aufruf wird abgelehnt, ein blockierter Sitzungsstart beendet die Sitzung. „Nach einem Werkzeugaufruf“ läuft erst, wenn der Aufruf erledigt ist, daher zeigt 2 dort nur seinen Einwand.
- Alles andere, ein abgelaufenes Zeitlimit oder ein Hook-Befehl, der nicht starten kann, ist ein Fehlschlag. Schlägt ein Wächter-Hook (Vor einem Werkzeugaufruf oder Vor einer Rückfrage) fehl, fragt der Aufruf nach, auch wo der Modus oder eine Erlaubnisregel ihn erlaubt hätte. Andere Hook-Befehle melden den Fehlschlag, und die Sitzung läuft weiter.

> Ein Hook-Befehl kann einen Aufruf blockieren, aber nie freigeben. Die Hook-Befehle eines Projekts in `.freeloop/hooks.yaml` laufen erst, wenn dem Projekt vertraut wird, und nur in der Fassung, die dabei zu sehen war oder seitdem freigegeben wurde.

## Eine Entscheidung prüfen und Wartendes freigeben

Unter Einstellungen → Richtlinie beurteilt **Wäre das erlaubt?** einen Befehl, eine Änderung, einen Lese- oder Abrufvorgang, ohne ihn auszuführen. Ort, Modus und Aktion wählen, Befehl, Pfad oder URL eingeben und **Prüfen** drücken, um das Urteil und seine Begründung zu sehen.

Ein Hook-Befehl, eine Erlaubnisregel, eine Umgebung, eine Einstellung, eine Wahl des Token-Optimierers oder eine MCP-Server-Adresse, die außerhalb der App hinzugekommen ist, etwa aus einer wiederhergestellten Sicherung oder durch Bearbeiten von Hand, bewirkt nichts, bis sie unter **Wartet auf Freigabe** in Einstellungen → Berechtigungen freigegeben wird. Die Hook-Befehle eines vertrauenswürdigen Projekts warten erneut, wenn sie sich ändern. Nur freigeben, was bekannt ist. Verbotsregeln brauchen keine Freigabe und gelten sofort.
