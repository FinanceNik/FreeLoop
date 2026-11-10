# Vaardigheden en MCP-servers

Vaardigheden geven agents extra kennis, MCP-servers extra hulpprogramma's.

## Wat een vaardigheid is

Een vaardigheid is een map met een bestand `SKILL.md` dat een agent uitlegt hoe één soort taak gaat, plus eventuele scripts en naslagbestanden. Ze gebruikt het open formaat Agent Skills, zodat vaardigheden die voor Claude Code zijn geschreven zonder aanpassing laden.

De agent ziet van elke ingeschakelde vaardigheid alleen de naam en de beschrijving, en laadt de volledige tekst als een taak erbij past.

## Vaardigheden installeren en beheren

**Instellingen → Vaardigheden** toont elke vaardigheid die Freeloop heeft gevonden. Ze staan in de vaardighedenopslag, de map `skills` in de gegevensmap van Freeloop, of in `.freeloop/skills`, `.agents/skills` of `.claude/skills` van een vertrouwd project. Freeloop wijzigt nooit projectmappen, en een projectvaardigheid verbergt een vaardigheid met dezelfde naam uit de opslag.

Installeren kopieert een vaardigheid naar de opslag en laat de bron ongemoeid:

- **Installeren uit een map…**, of een vaardigheidsmap of `.zip` op het neerzetvlak slepen.
- Een mappad, een `.zip`-pad of een git-URL plakken en op **Voorbeeld** klikken. Met `#branch`, `#tag` of `#commit` wordt een git-URL vastgezet.
- **Importeren uit Claude Code** leest `~/.claude/skills`. Er wordt niets teruggeschreven.

Elke route toont eerst een voorbeeld, waarin elke vaardigheid als nieuw, actueel, gewijzigd of niet te laden is gemarkeerd. De gewenste aanvinken en op de installatieknop klikken.

Elke vaardigheid heeft **Overal aan** en, als er een project is gekozen, **In dit project**. Een vaardigheid die uit staat, wordt niet aan het model getoond. Een profiel kan een sessie ook beperken tot bepaalde vaardigheden in zijn veld **Vaardigheden**.

Een vaardigheid uit git biedt **Op nieuwe versies controleren**, daarna **Bijwerken**, en **Terugrollen** zet de vorige versie terug. **Nieuwe vaardigheid…** begint met een sjabloon. **Testen…** vraagt een model één keer of het de vaardigheid zou kiezen voor een zelfgeschreven prompt; dat kost tokens.

> De scripts van een vaardigheid draaien nooit vanzelf. De ingebouwde agent voert ze alleen uit via het shell-hulpprogramma, zodat de modus en de regels over elk ervan beslissen zoals over elke andere opdracht. Ook Claude Code krijgt de ingeschakelde vaardigheden.

## Een MCP-server toevoegen

MCP-servers geven een agent extra hulpprogramma's. Ze draaien als opdracht op de eigen computer of worden via HTTP bereikt. In **Instellingen → MCP-servers** op **Server toevoegen** klikken en het formulier invullen.

| Veld | Betekenis |
| --- | --- |
| ID | Geeft de server een naam. De hulpprogramma's ervan verschijnen als `mcp__ID__tool`. |
| Verbindingstype | Opdracht (stdio) of Op afstand (Streamable HTTP). |
| Opdracht, Argumenten, Variabelen | Voor een opdrachtserver. Eén argument per regel. |
| URL, Headers | Voor een server op afstand. |
| Geheimen | Bewaard in {{de macOS-sleutelhanger|Windows Referentiebeheer|de sleutelbos van het systeem}} en doorgegeven als variabelen of verstuurd als headers. |
| De hulpprogramma's ervan in elke sessie aanbieden | Zet de server voor iedereen aan. Een profiel kan servers toevoegen die uit staan. |

De snelste start is **Suggesties**, verderop op dezelfde pagina: GitHub, Linear, Sentry, Notion, Context7 en Playwright. **Toevoegen…** vult het formulier in; opslaan en goedkeuren gaat zoals bij elke andere server. Elke kaart vermeldt waar de server draait en wat hij kan bereiken.

**Een register doorzoeken…** vult het formulier in vanuit het officiële MCP-register of een eigen adres. **Importeren uit Claude Code…** kopieert servers uit de instellingen van Claude Code.

## Goedkeuren en aanmelden

Een nieuwe server toont **goedkeuring nodig** en precies de opdracht of URL die hij zou uitvoeren. Op **Goedkeuren en starten** klikken. Een latere wijziging in wat hij uitvoert of waarmee hij verbindt, ook in variabelen, headers of namen van geheimen, vraagt opnieuw om goedkeuring.

Een server op afstand die OAuth wil, toont **aanmelding nodig**. Op **Aanmelden…** klikken en de aanmelding in de browser afronden. Tokens blijven in {{de macOS-sleutelhanger|Windows Referentiebeheer|de sleutelbos van het systeem}}, en **Afmelden** verwijdert ze.

> Servers draaien met de rechten van de ingelogde gebruiker. Opdrachten die pakketten ophalen, zoals `npx`, krijgen een waarschuwing; waar mogelijk een versie vastzetten.

## Servers in een sessie gebruiken

Hulpprogramma's bereiken de ingebouwde agent als `mcp__server__tool`. Elke aanroep vraagt om goedkeuring, tenzij een regel hem toestaat, bijvoorbeeld `mcp__github__*`, of de sessie in de modus Omzeilen staat. De modus Plannen blokkeert ze.

- **MCP-bron…** naast het berichtvak voegt een bron van een server toe aan het volgende bericht.
- De prompts van een server verschijnen als slash-opdrachten gemarkeerd met **MCP-prompt**.
- Externe agents krijgen de goedgekeurde servers die aan staan, plus die in het veld **MCP-servers** van het profiel. Servers op afstand gaan alleen naar agents die HTTP ondersteunen.
