# Spracheingabe

Einen Prompt sprechen statt tippen. Die Sprache wird auf diesem Computer zu Text.

## Diktieren

Das Mikrofon im Nachrichtenfeld anklicken oder ⌘⇧D drücken, und sprechen. Ein zweiter Klick oder ein zweites ⌘⇧D setzt die Wörter ins Nachrichtenfeld, wo der Cursor stand. Gesendet wird nichts: Den Text lesen, ändern und dann wie gewohnt senden. Escape verwirft die Aufnahme.

- Eine Aufnahme darf bis zu 3 Minuten lang sein.
- Beim ersten Mal fragt {{macOS|Windows|das System}}, ob Freeloop das Mikrofon verwenden darf.
- War nichts zu hören, wird nichts eingefügt. Dann etwas lauter sprechen oder das Mikrofon prüfen.

## Das Sprachmodell

Die Spracheingabe führt Whisper large-v3-turbo mit whisper.cpp aus, beide unter MIT-Lizenz. Es ist gut in Englisch, Deutsch und Niederländisch und in vielen weiteren Sprachen.

Beim ersten Diktat bittet Freeloop darum, das Modell einmal herunterzuladen: etwa 870 MB von Hugging Face, unter Windows und Linux dazu whisper.cpp von GitHub. Jede Datei ist festgelegt und wird gegen eine bekannte Prüfsumme geprüft, bevor sie behalten wird. **Einstellungen → Spracheingabe** zeigt, wo sie liegt, und entfernt sie. Im Nur-lokal-Modus wird der Download verweigert.

Wie schnell es geht, hängt vom Computer ab. Auf einem Mac mit Apple-Chip dauert ein Satz etwa eine Sekunde. Auf einem Computer ohne Grafikchip, den whisper.cpp nutzen kann, kann eine lange Aufnahme mehrere Sekunden brauchen.

## Welche Sprache

Standardmäßig teilt Freeloop dem Modell mit, dass in der Sprache des Fensters gesprochen wird. Unter **Einstellungen → Spracheingabe** lässt sich eine andere Sprache angeben oder jedes Mal erkennen. Sie anzugeben ist genauer: Ein kurzer niederländischer Satz wird leicht für Deutsch gehalten.

## Was auf diesem Computer bleibt

- Das Gesagte geht vom Mikrofon an whisper.cpp auf diesem Computer und wird weder auf die Festplatte geschrieben noch irgendwohin gesendet.
- whisper.cpp läuft nur während des Diktats und endet, sobald der Text zurück ist.

## Wenn es nicht funktioniert

- **Freeloop darf das Mikrofon nicht verwenden**: Unter {{Systemeinstellungen → Datenschutz & Sicherheit → Mikrofon|Einstellungen → Datenschutz und Sicherheit → Mikrofon, mit eingeschaltetem Zugriff für Desktop-Apps|den Datenschutz- oder Toneinstellungen des Systems}} erlauben und erneut versuchen.
- {{Nach einem Update fragt macOS womöglich erneut: Die Versionen von Freeloop sind nicht mit einem Apple-Zertifikat signiert.|Unter Windows braucht whisper.cpp das Microsoft Visual C++ Redistributable (2015–2022); Freeloop sagt es, wenn es fehlt.|Unter Linux braucht whisper.cpp glibc 2.35 oder neuer und libgomp1, und das Fenster braucht das PipeWire- oder PulseAudio-Plugin von GStreamer.}}
