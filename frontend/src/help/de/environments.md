# Umgebungen

Festlegen, wo die Befehle einer Sitzung laufen und was jede Wahl zurückhält.

Eine Umgebung ist das, womit die Befehle einer Sitzung laufen: Variablen, Geheimnisse, die Werkzeuge, die der Agent nutzen darf, und der Modus, in dem neue Sitzungen starten. Verwaltet werden Umgebungen unter Einstellungen → Umgebungen mit **Umgebung hinzufügen**, **Bearbeiten** und **Löschen** (**Zurücksetzen** bei einer eingebauten). Eingebaut sind „Local“ und „Local, sandboxed“.

## Eine Umgebung wählen

- Für eine einzelne Sitzung das Menü **Umgebung** im Formular für neue Sitzungen nutzen. Es zeigt an, ob Befehle auf diesem Computer nachfragen oder in einer Sandbox laufen.
- Als Vorgabe beginnt das Formular mit der Standardumgebung des Projekts oder, bei Projekten ohne eigene, mit der unter Einstellungen → Sitzungen gewählten.
- **Startmodus neuer Sitzungen** legt den Modus fest, in dem Sitzungen beginnen. Ein Profil mit eigenem Modus hat Vorrang.

Den stärksten Halt bei einem Repository, dem nicht zu trauen ist, bietet Docker. Für eine schnelle, ruhigere Sitzung am eigenen Code passt Local, sandboxed.

## Local

Befehle laufen auf diesem Computer, im Projektordner, mit allen Rechten des Benutzerkontos.

> Nichts schränkt sie ein. Die Berechtigungsprüfung fragt weiterhin, bevor ein Befehl läuft, aber ein freigegebener Befehl kann alles lesen und ändern, was das eigene Konto kann. Local ist keine Sandbox.

Standardmäßig können Befehle, Agenten und Terminals einer lokalen Umgebung den eigenen SSH-Agenten nutzen, sodass `git push` und `git pull` über SSH funktionieren. Das heißt aber auch: Ein freigegebener Befehl kann die eigenen SSH-Schlüssel für alles andere nutzen, etwa zum Anmelden an einem Server.

Um das zu verhindern, in der Umgebung **Befehle und Agenten dürfen den eigenen SSH-Agenten nutzen** abwählen. Git über SSH braucht dann einen anderen Zugang, etwa HTTPS.

## Local, sandboxed

Befehle laufen unter {{der macOS-Sandbox|einer Sandbox, die Windows noch nicht hat: Unter Windows weist diese Umgebung darauf hin und führt nichts aus, für eingeschränkte Befehle also eine Docker-Umgebung nehmen|bubblewrap (siehe Fehlerbehebung, falls es nicht startet)}}. Sie können nur im Projekt schreiben und außerhalb wenig lesen, daher laufen die Befehle des eingebauten Agenten ohne Rückfrage, und Verbotsregeln haben weiterhin Vorrang. Vor dem Verlassen der Sandbox kommt jedes Mal eine Rückfrage. Lässt sich die Sandbox nicht anwenden, läuft der Befehl nicht.

Darin gilt für Befehle:

- Schreiben nur im Projekt, in seinen Caches und in temporären Dateien
- kein Lesen des Home-Ordners (außer gängiger Toolchains), von Shell-Startdateien oder der eigenen Konfiguration von git
- kein Schreiben in `.git/config`, Git-Hooks, verschachtelte `.git`- oder `.freeloop`-Ordner oder andere Dateien, nach denen Werkzeuge später handeln
- standardmäßig nur das Netzwerk dieses Computers selbst erreichbar

**Netzwerk** lässt sich auf **Nur dieser Computer**, **Dieser Computer und die hier aufgeführten Hosts** oder **Keines** stellen. Mit einer Host-Liste muss ein Werkzeug den Proxy nutzen, den die Sandbox einrichtet. Außerdem lassen sich weitere Ordner zum Lesen oder Schreiben hinzufügen.

> Die Sandbox deckt weder die eigenen Werkzeuge eines externen Agenten noch MCP-Server noch Eingaben im Terminal einer Sitzung ab. Darin ist Lesen außerhalb des Home-Ordners offen, `/tmp` wird geteilt, und Nur dieser Computer erreicht weiterhin alles andere, was auf diesem Computer lauscht. Ein Befehl kann weiterhin eine Datei schreiben, die später von Hand ausgeführt wird, etwa ein `Makefile`.

## Docker

Jede Sitzung bekommt ihren eigenen Container, in dem der Projektordner unter demselben Pfad geteilt ist. Befehle und externe Agenten laufen darin, sehen also das Projekt und das Image, nicht die übrigen eigenen Dateien.

- Ein Image angeben oder die `.devcontainer/devcontainer.json` des Projekts nutzen; dafür muss das Projekt vertrauenswürdig sein.
- Ein **Einrichtungsbefehl** läuft einmal, nachdem der Container erstellt wurde, und eine **Funktionsprüfung** vor jedem Sitzungsstart.
- Nötig ist Docker Desktop oder Colima mit der docker CLI.

> Ein Container ist kein Tresor. Er hat Netzwerkzugang, läuft als Benutzer des Images (oft root) und kann alles im geteilten Projektordner ändern. Ausnahmen sind schreibgeschützt eingebunden: der `.freeloop`-Ordner des Projekts, die Devcontainer-Dateien sowie `.git/config` und die Git-Hooks des Repositorys. Der Rest von `.git` bleibt beschreibbar, daher mit einem solchen Checkout im eigenen Terminal vorsichtig umgehen.

## SSH

Befehle laufen auf einer anderen Maschine, in einer Arbeitskopie, die mit dem Projekt auf diesem Computer synchron gehalten wird. Als **Host** entweder einen Namen aus `~/.ssh/config` oder `user@host` eingeben.

Vorher einmal in einem Terminal `ssh` zu diesem Host ausführen und seinen Fingerabdruck prüfen, denn Freeloop akzeptiert keinen Host-Schlüssel von sich aus. Die Anmeldung läuft über die eigenen Schlüssel und den ssh-Agenten; Freeloop liest sie nie.

Der anderen Maschine wird nicht vertraut:

- Geheimnisse, Dateien, die unter eine Verbotsregel fallen, und was das Projekt ignoriert, werden nicht dorthin kopiert.
- Nur reguläre Dateien kommen zurück. Einstellungen, nach denen dieser Computer oder ein Agent handeln würde, etwa `.git`, `.claude` und `AGENTS.md`, kommen nicht zurück.
- Andere Änderungen kommen sehr wohl zurück, daher vor dem Ausführen von irgendetwas den Diff lesen.
- Der Code sowie die Variablen und Geheimnisse dieser Umgebung gehen an diese Maschine.

Nur eine Maschine nehmen, deren Verlust verkraftbar ist. Bisher funktioniert eine SSH-Umgebung nur mit dem eingebauten Agenten.

## Variablen und Geheimnisse

- **Variablen** sind ein `KEY=value` pro Zeile und werden der Umgebung jedes Befehls hinzugefügt.
- **Geheimnisse** kommen über **Geheimnis hinzufügen** dazu. Sie werden in {{dem macOS-Schlüsselbund|der Windows-Anmeldeinformationsverwaltung|dem Schlüsselbund des Systems}} gespeichert, Befehlen beim Start als Variablen übergeben und aus Transkripten entfernt.

Das Entfernen erfasst Werte, die Freeloop kennt. Text, den ein Modell bereits erhalten hat, ändert sich nicht; ein Geheimnis, das ein Befehl ausgibt, kann also das Modell erreichen.

Eine von Hand in ihrer YAML-Datei bearbeitete Umgebung wartet auf Freigabe unter Einstellungen → Berechtigungen, bevor eine Sitzung sie nutzen kann.
