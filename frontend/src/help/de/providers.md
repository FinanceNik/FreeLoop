# Anbieter und Modelle

Den eingebauten Agenten mit einem Modell verbinden und dann das Modell einer Sitzung wählen und wechseln.

## Einen Anbieter hinzufügen

Ein Anbieter ist das Ziel der Modellaufrufe des eingebauten Agenten: ein API-Key für einen gehosteten Dienst oder die Adresse eines Modells auf dem eigenen Rechner.

- **Einstellungen → Anbieter** öffnen und **Anbieter hinzufügen** wählen.
- Die Art wählen und einen Namen vergeben.
- Den API-Key eingeben. Für lokale Server ist er optional.
- Wahlweise ein Standardmodell festlegen, dann **Speichern** wählen.

Freeloop testet die Verbindung beim Speichern. Ein funktionierender Anbieter zeigt **Verbunden** und die Zahl der gelisteten Modelle, ein Fehlschlag zeigt den Fehler. Mit **Verbindung testen** läuft der Test erneut.

Externe Agenten wie Claude Code nutzen keine Anbieter. Sie bringen ihre eigene Anmeldung mit und erscheinen auf derselben Seite unter Angemeldete Agenten.

## Eine Art wählen

Die Wahl einer Art füllt die Basis-URL aus, bei Anthropic, OpenAI (Responses API) und Google Gemini auch ein Standardmodell.

| Art | Hinweise |
| --- | --- |
| Anthropic | Braucht einen API-Key. |
| OpenAI (Responses API) | Die native API von OpenAI. Sie behält Zusammenfassungen des Denkens und das Denken über Werkzeugaufrufe hinweg. |
| Google Gemini | Braucht einen API-Key aus Google AI Studio. |
| OpenAI | OpenAI über die Chat Completions API. |
| OpenRouter | Nicht jedes Modell dort unterstützt Werkzeuge. Eines wählen, das es tut. |
| Ollama (lokal) | Zuerst ein Modell mit Werkzeugunterstützung laden, zum Beispiel `ollama pull qwen3`. |
| LM Studio (lokal) | Ein lokaler Server. |
| llama.cpp server (lokal) | `llama-server` mit `--jinja` starten, sonst funktionieren Werkzeugaufrufe nicht. |
| vLLM (lokal) | Mit `--enable-auto-tool-choice` und einem `--tool-call-parser` starten. |
| Ollama (OpenAI-compatible) (lokal) | Die native Art Ollama ist meist die bessere Wahl. |
| Custom OpenAI-compatible (lokal) | Jeder Server, der die OpenAI-API spricht. Die Basis-URL wird von Hand eingetragen. |

## Schlüssel und Adressen

API-Keys liegen in {{dem macOS-Schlüsselbund|der Windows-Anmeldeinformationsverwaltung|dem Schlüsselbund des Systems}}, und Freeloop zeigt einen gespeicherten Schlüssel nie wieder an. Beim Bearbeiten eines Anbieters bleibt der Schlüssel erhalten, wenn das Feld leer bleibt. **Löschen** entfernt den Anbieter und seinen Schlüssel.

> Freeloop verbindet sich nur mit den hinzugefügten Endpunkten. Wird die Adresse eines Anbieters außerhalb der App geändert, wird der Schlüssel nicht gesendet und der Anbieter zeigt eine Warnung. **Diese Adresse verwenden** nur wählen, wenn die Änderung von einem selbst stammt.

## Ein Modell wählen

Das Feld **Modell** im Formular für neue Sitzungen listet die Modelle, die der Endpunkt meldet. Es lässt sich auch jede beliebige Modell-ID eintippen, um ein Modell zu nutzen, das der Endpunkt nicht listet.

- **☆ Favorit** setzt das gewählte Modell an den Anfang der Liste.
- **Ausblenden** lässt es weg. Mit **Ausgeblendete zeigen** erscheinen ausgeblendete Modelle, und Einblenden holt eines zurück.

## Werkzeugaufrufe prüfen

Der eingebaute Agent arbeitet mit Werkzeugen, also muss ein Modell sie aufrufen können. Unter dem Modell zeigt Freeloop, was es weiß:

- Werkzeugaufrufe unterstützt: für diesen Anbieter bekannt, vom Server gemeldet, getestet oder von Hand gesetzt.
- Ob das Modell Werkzeugaufrufe beherrscht, ist noch nicht bekannt: **Jetzt testen** wählen, das bittet das Modell, ein triviales Werkzeug aufzurufen. Lokale Modelle brauchen zum Laden womöglich eine Minute. Ohne Test wird beim Start der Sitzung getestet.
- Der eingebaute Agent kann dieses Modell nicht nutzen: Das Modell hat mit Text geantwortet oder der Server meldet keine Werkzeugunterstützung, daher ist Sitzung starten gesperrt.

**Trotzdem nutzen** übergeht das Ergebnis, und Überschreibung zurücknehmen hebt das wieder auf.

> Ein Modell, das Werkzeuge nicht wirklich aufrufen kann, scheitert während der Sitzung.

## Das Modell während einer Sitzung wechseln

In einer Sitzung mit dem eingebauten Agenten ⌘K drücken und **Modell wechseln…** wählen. Unter **Nächste Runde nutzt** einen Anbieter und ein Modell wählen, dann **Wechseln** wählen.

Der nächste Modellaufruf nutzt das neue Modell mit dem ganzen Gespräch, und das Transkript vermerkt den Wechsel. Das neue Modell muss Werkzeuge aufrufen können. Externe Agenten wählen ihr Modell selbst.
