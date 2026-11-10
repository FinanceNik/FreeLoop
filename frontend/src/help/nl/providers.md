# Aanbieders en modellen

De ingebouwde agent met een model verbinden, en daarna het model van een sessie kiezen en wijzigen.

## Een aanbieder toevoegen

Een aanbieder is waar de ingebouwde agent zijn modelaanroepen naartoe stuurt: een API-key voor een gehoste dienst, of het adres van een model op de eigen computer.

- **Instellingen → Aanbieders** openen en op **Aanbieder toevoegen** klikken.
- De soort kiezen en een naam geven.
- De API-key invoeren. Voor lokale servers is die optioneel.
- Eventueel een standaardmodel instellen en dan op **Opslaan** klikken.

Freeloop test de verbinding bij het opslaan. Een werkende aanbieder toont **Verbonden** en het aantal gevonden modellen, een mislukking toont de fout. Met **Verbinding testen** loopt de test opnieuw.

Externe agents zoals Claude Code gebruiken geen aanbieders. Ze brengen hun eigen aanmelding mee en staan op dezelfde pagina onder Aangemelde agents.

## Een soort kiezen

Het kiezen van een soort vult de basis-URL in, en voor Anthropic, OpenAI (Responses API) en Google Gemini ook een standaardmodel.

| Soort | Opmerkingen |
| --- | --- |
| Anthropic | Vraagt een API-key. |
| OpenAI (Responses API) | De eigen API van OpenAI. Die bewaart samenvattingen van de redenering en de redenering zelf over tool-aanroepen heen. |
| Google Gemini | Vraagt een API-key uit Google AI Studio. |
| OpenAI | OpenAI via de Chat Completions API. |
| OpenRouter | Niet elk model daar ondersteunt hulpprogramma's. Een model kiezen dat dat wel doet. |
| Ollama (lokaal) | Eerst een model met ondersteuning voor hulpprogramma's binnenhalen, bijvoorbeeld `ollama pull qwen3`. |
| LM Studio (lokaal) | Een lokale server. |
| llama.cpp server (lokaal) | `llama-server` starten met `--jinja`, anders werken tool-aanroepen niet. |
| vLLM (lokaal) | Starten met `--enable-auto-tool-choice` en een `--tool-call-parser`. |
| Ollama (OpenAI-compatible) (lokaal) | De eigen soort Ollama is meestal de betere keuze. |
| Custom OpenAI-compatible (lokaal) | Elke server die de OpenAI-API spreekt. De basis-URL wordt met de hand ingevuld. |

## Sleutels en adressen

API-keys worden bewaard in {{de macOS-sleutelhanger|Windows Referentiebeheer|de sleutelbos van het systeem}}, en Freeloop toont een opgeslagen sleutel nooit meer. Bij het bewerken van een aanbieder blijft de sleutel behouden als het veld leeg blijft. **Verwijderen** verwijdert de aanbieder en zijn sleutel.

> Freeloop maakt alleen verbinding met de toegevoegde eindpunten. Als het adres van een aanbieder buiten de app wordt gewijzigd, wordt de sleutel niet verstuurd en toont de aanbieder een waarschuwing. Alleen op **Dit adres gebruiken** klikken als die wijziging zelf is aangebracht.

## Een model kiezen

Het veld **Model** in het formulier voor een nieuwe sessie toont de modellen die het eindpunt meldt. Elke model-ID kan ook worden getypt, om een model te gebruiken dat het eindpunt niet vermeldt.

- **☆ Favoriet** zet het gekozen model bovenaan de lijst.
- **Verbergen** laat het weg. Met **Verborgen tonen** verschijnen verborgen modellen, en Tonen haalt er een terug.

## Tool-aanroepen controleren

De ingebouwde agent werkt met hulpprogramma's, dus een model moet die kunnen aanroepen. Onder het model toont Freeloop wat het weet:

- Tool-aanroepen ondersteund: bekend voor deze aanbieder, gemeld door de server, getest of handmatig ingesteld.
- Of het model tool-aanroepen ondersteunt, is nog niet bekend: op **Nu testen** klikken, wat het model vraagt een triviaal hulpprogramma aan te roepen. Lokale modellen kunnen een minuut nodig hebben om te laden. Zonder test wordt er getest als de sessie start.
- De ingebouwde agent kan dit model niet gebruiken: het model antwoordde met tekst of de server meldt geen ondersteuning voor hulpprogramma's, dus Sessie starten is uitgeschakeld.

**Toch gebruiken** gaat voorbij aan de uitkomst, en Overschrijving ongedaan maken draait dat terug.

> Een model dat niet echt hulpprogramma's kan aanroepen, faalt tijdens de sessie.

## Tijdens een sessie van model wisselen

In een sessie met de ingebouwde agent op ⌘K drukken en **Ander model kiezen…** kiezen. Onder **Volgende beurt gebruikt** een aanbieder en een model kiezen en dan op **Wisselen** klikken.

De volgende modelaanroep gebruikt het nieuwe model met het hele gesprek, en het transcript legt de wissel vast. Het nieuwe model moet hulpprogramma's kunnen aanroepen. Externe agents kiezen hun eigen model.
