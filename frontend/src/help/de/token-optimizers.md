# Token-Optimierer

Shell-Ausgabe kürzen, bevor das Modell sie sieht, ohne die Schutzmechanismen aufzugeben.

## Was er tut

Die Ausgabe eines Shell-Befehls geht zurück an das Modell und kostet Token. Ein Token-Optimierer wie RTK ist ein kleines Programm, das sie vorher kürzt.

Kürzere Ausgabe kann etwas auslassen, das der Agent braucht, etwa eine Commit-Nachricht oder das Ende eines großen Diffs, daher sind Optimierer aus, bis einer gewählt wird. Sie funktionieren mit dem eingebauten Agenten und Claude Code, aber nicht mit anderen externen Agenten, SSH-Umgebungen oder Claude Code in einer Docker-Umgebung.

## Einen einschalten

**Einstellungen → Token-Optimierer** öffnen. Gewählt werden kann an vier Stellen:

- **Für neue Sitzungen** legt die Vorgabe fest. **Aus** ist die Ausgangswahl.
- Ein Profil hat ein eigenes Feld **Token-Optimierer**.
- Das Formular für neue Sitzungen hat eine Liste, deren erster Eintrag dem Profil oder der Vorgabe folgt.
- Eine offene Sitzung hat eine Liste in ihrer Kopfzeile, oder **Token-Optimierer: ...** in der Befehlspalette.

Ein Optimierer, der nicht installiert ist, zeigt **(nicht installiert)**.

## RTK installieren

Liegt RTK im PATH, findet Freeloop es. Andernfalls zeigt **RTK installieren** vor dem Klick die Version, die Lizenz und die Quelle (die GitHub-Releases von RTK).

- Der Download ist auf eine Version festgelegt und wird gegen eine feste Prüfsumme geprüft.
- Er startet nur per Klick, und **Nur lokal** unter **Einstellungen → Netzwerk** verweigert ihn.
- Die Kopie bleibt im eigenen Ordner von Freeloop. **Kopie von Freeloop entfernen** löscht sie.

Für ein selbst installiertes Programm zeigt die Seite einen Installationsbefehl mit einer Schaltfläche **Kopieren**. **Prüfen** führt das Programm einmal aus und meldet, ob es funktioniert.

## Eigene hinzufügen

Eine Datei in den Ordner `optimizers` des Datenordners legen. Freeloop listet sie als **(nicht freigegeben)** auf und zeigt, was sie für jeden Befehl ausführen würde. **Freigeben** nur anklicken, wenn ihr zu trauen ist.

Dasselbe gilt für eine Wahl, die aus einer Datei kommt. Nennt eine Konfigurations- oder Profildatei einen Optimierer, starten neue Sitzungen mit ausgeschaltetem Optimierer, bis er unter **Einstellungen → Berechtigungen** freigegeben wird. Das Profil eines Projekts kann keinen einschalten.

## Was sicher bleibt

- Die Freigabe kommt zuerst. Der Berechtigungsvermittler beurteilt den Befehl so, wie das Modell ihn geschrieben hat, und erst dann wird der Optimierer nach einer kürzeren Form gefragt.
- Nur einfache Befehle werden umgeschrieben. Eine Pipe, Umleitung oder Ersetzung läuft wie geschrieben.
- Gibt der Optimierer etwas anderes zurück als denselben Befehl in seiner eigenen Form, oder fehlt er, ist er langsam oder schlägt fehl, läuft das Original.
- Das Auflisten führt nichts aus. Freeloop führt das Programm erst aus, wenn es gewählt oder geprüft wird, und verweigert eines, das im Projektordner gefunden wird.

> Einen Optimierer zu wählen heißt weiterhin, diesem Programm zu vertrauen. Es läuft mit den Rechten der Sitzung.

> Hat RTK einen eigenen Hook in der Einstellungsdatei von Claude Code, schreibt dieser Hook die Befehle von Claude Code um, egal was hier gewählt ist, und er kann Befehle ohne Rückfrage freigeben. Freeloop ändert diese Datei nicht; eine Sitzung zeigt eine Warnung, wenn sie einen solchen Hook findet.
