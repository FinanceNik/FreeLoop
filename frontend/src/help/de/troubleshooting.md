# Fehlerbehebung

Abhilfe, wenn die App nicht startet, ein Agent oder Anbieter scheitert oder eine Sitzung hängt, und wie sich ein Problem melden lässt.

## Die App startet nicht

{{Freeloop-Releases sind weder signiert noch notarisiert, deshalb blockiert macOS eine heruntergeladene Kopie beim ersten Mal. Erst versuchen, sie zu öffnen, dann unter Systemeinstellungen → Datenschutz & Sicherheit **Dennoch öffnen** wählen. Eine selbst gebaute App öffnet sich normal.

Die App braucht einen Mac mit Apple Silicon und macOS 14 oder neuer. Nach jedem Update fragt der Schlüsselbund erneut nach Zugriff auf die Schlüssel, die die App gespeichert hat. Das ist zu erwarten: Die Signatur ändert sich mit jedem Build.|Freeloop-Releases sind nicht signiert, deshalb kann Windows SmartScreen eine heruntergeladene Kopie beim ersten Mal anhalten. **Weitere Informationen** und dann **Trotzdem ausführen** wählen. Eine selbst gebaute App öffnet sich normal.

Die App braucht Windows 10 Version 1809 oder neuer mit der Microsoft Edge WebView2 Runtime, die Windows 11 mitbringt und die der Installer sonst anbietet. Befehle, Hooks und das Terminal laufen über Git Bash: Meldet ein Befehl, dass es fehlt, Git for Windows von git-scm.com installieren und Freeloop neu starten.|Die App braucht WebKitGTK und GTK aus der Distribution. Startet sie nicht, zeigt ein Start aus einem Terminal, was fehlt; dann das WebKitGTK-Paket der Distribution installieren.

Schlüssel gehen über den Secret Service in den Schlüsselbund des Systems, etwa GNOME Keyring oder KWallet; zum Speichern eines Schlüssels muss einer davon laufen.}}

## Ein Agent startet nicht

Einstellungen → Anbieter öffnen. Unter **Angemeldete Agenten** zeigt jeder externe Agent **bereit** oder den Grund, warum er nicht starten kann.

- „Is not on your login PATH“ heißt, dass der Befehl des Agenten nicht installiert ist oder die Login-Shell ihn nicht findet. Den Agenten installieren, bei Node.js-Agenten zuerst Node.js.
- Ein externer Agent meldet sich selbst an, und Freeloop sieht diese Anmeldung nie. Den Agenten einmal in einem Terminal starten und dort anmelden, für Claude Code etwa mit `claude`.
- Eine von Hand bearbeitete Agent-Vorlage startet erst, wenn sie unter Einstellungen → Berechtigungen bei **Wartet auf Freigabe** freigegeben ist.

Startet die Umgebung einer Sitzung nicht, auch diese prüfen:

- Docker braucht ein laufendes Docker, und eine fehlschlagende **Funktionsprüfung** blockiert die Sitzung.
- SSH lehnt einen unbekannten Host-Schlüssel ab. Einmal in einem Terminal per `ssh` zum Host verbinden und den Fingerabdruck prüfen.
- Eine Umgebung mit Sandbox führt keinen Befehl aus, wenn sich die Sandbox nicht anwenden lässt.

## Ein Anbieter zeigt einen Fehler

Unter Einstellungen → Anbieter neben dem Anbieter **Verbindung testen** wählen. Angezeigt wird **Verbunden** mit der Zahl der Modelle oder der Fehler, den der Anbieter zurückgegeben hat.

- Schlüssel, Adresse und Modellnamen prüfen.
- Bei einem lokalen Server wie Ollama oder LM Studio prüfen, ob er läuft.
- Ist **Nur lokal** unter Einstellungen → Netzwerk an, sind Anbieter anderswo nicht erreichbar.

## Eine Sitzung hängt

Eine Sitzung, die nicht vorankommt, zeigt ein Banner, das mit „Kommt nicht voran“ beginnt und den Grund nennt: Ein Kontolimit ist erreicht (und wann es zurückgesetzt wird), das Rundenlimit ist erreicht, dasselbe Werkzeug ist dreimal hintereinander gescheitert, es gab zwei Rückspulungen in einer Runde, oder zwei Runden hintereinander endeten mit einem Fehler.

Mit der Schaltfläche **Auf … fortfahren** im Banner oder dem Menü **Anderes Profil…** geht es in einer neuen Sitzung im selben Projekt weiter, mit einem anderen Profil, etwa einem mit einem anderen Anbieter oder Agenten. Die letzte Anfrage der alten Sitzung und eine Zusammenfassung davon gehen mit.

> **Auf … fortfahren** nennt das Profil, das auf der Seite **Profile** unter **Wenn eine Sitzung nicht vorankommt** für diesen Agenten gewählt ist. Ist ein Kontolimit erreicht und setzt das Profil der Sitzung `onLimit`, sagt das Banner, dass es nach dem Ende der Runde auf diesem Profil weitergeht, und das geschieht ohne Rückfrage.

## Die Sandbox startet unter Linux nicht

Unter Linux führt **Local, sandboxed** Befehle unter bubblewrap aus. Meldet die Sitzung, dass es fehlt, das Paket `bubblewrap` installieren.

Bubblewrap braucht unprivilegierte User-Namespaces. Ubuntu 24.04 und neuer verweigert sie über AppArmor allen Programmen ohne eigenes Profil, deshalb meldet die Sitzung, dass die Sandbox nicht starten würde. Um bubblewrap ein Profil zu geben, Folgendes als `/etc/apparmor.d/bwrap` speichern:

```
abi <abi/4.0>,
include <tunables/global>

profile bwrap /usr/bin/bwrap flags=(unconfined) {
  userns,
  include if exists <local/bwrap>
}
```

Dann das Profil mit `sudo apparmor_parser -r /etc/apparmor.d/bwrap` laden und die Sitzung neu starten.

> Damit darf jedes Programm, das bubblewrap ausführt, User-Namespaces anlegen, wie es auch Flatpak tut; Ubuntu schränkt sie ein, um manche Angriffe auf den Kernel zu erschweren. Wer das nicht möchte, kann Befehle stattdessen mit einer Docker-Umgebung eingrenzen.

## Konfigurationsprobleme und Protokolle

Lässt sich eine Einstellungsdatei nicht lesen, erscheint ein Banner: „Probleme in der Konfiguration; wo nötig gelten Standardwerte.“ Jede Zeile nennt Datei, Zeile und Feld. Den Fehler beheben und speichern. Freeloop übernimmt Änderungen innerhalb einer Sekunde, und das Banner verschwindet, sobald das Problem behoben ist. Was auf Freigabe wartet, gilt nicht als Problem.

Das Protokoll der App liegt unter {{`~/Library/Application Support/freeloop/logs/freeloop.log`|`%AppData%\freeloop\logs\freeloop.log`|`~/.config/freeloop/logs/freeloop.log`}}. Es bleibt auf diesem Computer. Unter Einstellungen → Daten öffnet **Protokolle {{im Finder|im Explorer|im Dateimanager}} zeigen** den Ordner.

## Ein Problem melden

Unter Einstellungen → Daten legt **Bericht kopieren** im Abschnitt **Problem melden** eine kurze Diagnose in die Zwischenablage. **Zeigen, was kopiert wird** zeigt sie vorher an.

Der Bericht enthält die Version von Freeloop, Systemversion, Modell, Chip und Arbeitsspeicher dieses Computers, Agent, Anbieter, Modell, Modus und Status der offenen Sitzung (oder der zuletzt offenen) und die letzten 200 Zeilen des Protokolls. Gespräche, Dateien, Sitzungstitel und Schlüssel bleiben draußen. Geheimnisse, die die App kennt, werden entfernt, und der Pfad des Benutzerordners wird als `~` geschrieben.

> Protokollzeilen können trotzdem Projekte nennen, deshalb den Bericht vor dem Weitergeben durchlesen.

Nichts wird irgendwohin gesendet. Der Bericht wird von Hand in ein Issue auf der GitHub-Seite des Projekts eingefügt. Bei einem Sicherheitsproblem dort statt eines öffentlichen Issues ein privates Security Advisory anlegen.
