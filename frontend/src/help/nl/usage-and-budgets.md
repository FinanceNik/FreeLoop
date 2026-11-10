# Gebruik, prijzen en budgetten

Zien wat de sessies verbruiken en kosten, en budgetten instellen die waarschuwen.

## De gebruiksweergaven openen

De knop **Gebruik** in de zijbalk opent de gebruikspagina. Naar een paneel gesleept, blijft het gebruik naast de sessies in beeld. In een paneel bladeren pijlen door Afgelopen uren, Afgelopen 12 maanden, Tokens en Per model.

Zodra iets dat vandaag is verbruikt een prijs heeft, of een budget bijna is bereikt, toont de zijbalk ook de geschatte kosten van vandaag.

## De grafieken lezen

| Tabblad | Toont |
| --- | --- |
| Afgelopen uren | De laatste 12 of 24 uur, met elke modelaanroep zodra die klaar is. |
| Tokens en kosten | 7 dagen tot 12 maanden, vergeleken met de periode ervoor. |
| Activiteit, Goedkeuringen, Efficiëntie | Wat agents deden, wie over elke goedkeuring besliste, wat een beurt kost. |

Op **Tokens en kosten** splitst **Groeperen op** de grafiek naar soort token, model, aanbieder of agent, of project. Filters gelden voor elk tabblad, en **Filters wissen** verwijdert ze. **CSV exporteren…** slaat de gegevens op.

## Wat geschat betekent

Freeloop telt de tokens van elke modelaanroep, ook die van externe agents die ze melden. Kosten zijn een schatting: tokens maal een prijs in Amerikaanse dollars per miljoen tokens.

Prijzen komen uit **Instellingen → Kosten**. Modellen van Anthropic, OpenAI en Google gebruiken de officiële catalogusprijzen van hun API. Een regel die onder Prijzen wordt toegevoegd gaat voor, zodat een eigen prijs kan worden ingevoerd, of een prijs voor een model dat niet in de lijst staat. Een model kan een patroon zijn zoals `claude-*`, en de eerste passende regel geldt.

- Een model zonder prijs telt als gratis. De pagina vermeldt hoeveel tokens dat zijn en biedt **Prijzen toevoegen** aan.
- Sommige agents melden voor een beurt geen tokens, of alleen een totaal. Een opmerking vermeldt welke, en de kosten laten die beurten weg.
- Een agent die wel tokens meldt maar niet zijn model, zoals Gemini CLI, kan toch een prijs krijgen. Daarvoor **Model waarop de agent draait** instellen in zijn profiel op de pagina Profielen.
- Een bedrag waarin iets ontbreekt, eindigt op **+**.

> Als er in de periode niets een prijs heeft, blijft de keuze Kosten uitgeschakeld tot er een prijs is toegevoegd.

## Limieten van agents

Het onderdeel Limieten toont hoeveel van een accountlimiet een agent als verbruikt heeft gemeld, wanneer die opnieuw begint en hoe oud de melding is. Een limiet kan **Dicht bij de limiet** of **Limiet bereikt** aangeven.

Freeloop vraagt geen enkele leverancier om deze cijfers. Ze komen mee met de eigen modelaanroepen van een agent, dus een limiet verschijnt pas na de volgende aanroep van de agent. API-keys en lokale modellen melden er geen.

## Budgetten instellen

Een budget is een bedrag aan geschatte uitgaven. Onder **Instellingen → Kosten → Budgetten** op **Budget toevoegen** klikken en instellen:

| Veld | Betekenis |
| --- | --- |
| Periode | Per dag of Per maand, in de lokale tijd. |
| Bedrag in Amerikaanse dollars | De limiet. |
| Voor | Elke sessie, of één profiel. |

Een budget telt alleen gebruik met een prijs, en de regel ervan vermeldt wat het weglaat. Elk budget toont een meter op het tabblad Tokens en kosten.

- Vanaf 80% markeren de gebruikspagina en de zijbalk het budget als bijna bereikt, en als meldingen aan staan komt er één melding per periode.
- Zodra het bedrag is bereikt, gebeurt hetzelfde nog eens en start de wachtrij onder dat budget geen nieuwe taak meer.
- De wachtrij wacht ook als de limiet van een agent dichtbij of bereikt is.

> Lopende sessies worden nooit gestopt. Een budget waarschuwt en houdt de wachtrij tegen, meer niet.
