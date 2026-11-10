# Privacy en gegevens

Wat Freeloop op de computer bewaart, wat het naar modelaanbieders stuurt en waarmee het verder verbinding kan maken.

## Wat op de computer blijft

Freeloop bewaart zijn gegevens in één map, {{`~/Library/Application Support/freeloop/`|`%AppData%\freeloop\`|`~/.config/freeloop/` (of onder `$XDG_CONFIG_HOME`)}}, die alleen het eigen gebruikersaccount kan openen. Alles daarin is leesbaar en blijft op deze computer. Er is geen telemetrie, geen analyse en geen crashrapportage.

- `sessions/<id>/events.jsonl`: het transcript van elke sessie
- `config.yaml`, `providers.yaml`, `rules.yaml`, `mcp.yaml` en de mappen `profiles`, `environments`, `presets`, `optimizers` en `skills`: instellingen en vaardigheden
- `freeloop.db`: de index van sessies en projecten
- `logs/freeloop.log`: het logboek van de app

API-keys, MCP-aanmeldtokens en geheimen van omgevingen staan in {{de macOS-sleutelhanger|Windows Referentiebeheer|de sleutelbos van het systeem}}. Bestanden zoals `providers.yaml` bevatten alleen verwijzingen ernaar, en sleutels van aanbieders komen nooit in de omgeving van een opdracht terecht.

> Transcripten zijn platte tekst, alleen beschermd door bestandsrechten. Freeloop vervangt geheimen die het kent voordat het een transcript opslaat. Niet op te vangen zijn een wachtwoord dat in een bericht wordt getypt, een sleutel die een opdracht van elders afdrukt, of een geheim korter dan acht tekens.

## Wat naar een aanbieder gaat

Als de ingebouwde agent een model aanroept, ontvangt de gekozen aanbieder het gesprek, de systeemprompt, de definities van de hulpprogramma's en de index van vaardigheden. Daartoe horen bestandsinhoud die de agent heeft gelezen, uitvoer van opdrachten en bestanden die met `@` zijn bijgevoegd.

Wat een aanbieder met de gegevens doet, regelen zijn eigen voorwaarden. Een model dat op de eigen computer draait, zoals in Ollama of LM Studio, houdt ze daar. Wat er naar buiten gaat, staat op het tabblad **Info** van een sessie onder **Wat het model ziet**.

> Een externe agent stelt zijn verzoeken zelf samen. Freeloop ziet alleen de berichten en aanroepen van hulpprogramma's, dus „Wat het model ziet” blijft voor die sessies leeg. De agent praat met zijn leverancier onder zijn eigen aanmelding, en Freeloop leest of bewaart die aanmelding nooit.

## Andere verbindingen

Naast wat wordt ingesteld of gevraagd, zoals aanbieders, MCP-servers, agents, SSH-machines, het git-adres van een vaardigheid of een pagina die de agent met goedkeuring ophaalt, maakt Freeloop verbinding met drie dingen, en alleen als ze worden gebruikt:

- De updatecontrole staat uit tot **Eén keer per dag op een nieuwere versie controleren** wordt aangezet in Instellingen → Netwerk. Ze vraagt GitHub naar de nieuwste uitgave en stuurt niets over deze kopie van de app. **Nu controleren** voert haar één keer uit.
- De download van een token-optimalisator gebeurt alleen bij een klik op de installatieknop ervan, zoals **RTK installeren**, in Instellingen → Token-optimalisatoren. Hij komt van GitHub, in een vastgezette versie, en wordt gecontroleerd tegen een bekende checksum.
- Het MCP-register wordt alleen benaderd als het wordt doorzocht via **Een register doorzoeken…** op de pagina MCP-servers. Het krijgt de zoekwoorden op het gekozen registeradres.

Als in Instellingen → Netwerk een zoekadres is ingesteld voor het zoeken op het web door de ingebouwde agent, gaan zoekopdrachten daarheen. Elke zoekopdracht vraagt eerst toestemming, tenzij een toestaanregel of de omzeilmodus hem doorlaat.

## Alleen-lokaal-modus

In Instellingen → Netwerk beperkt **Alleen lokaal: de app maakt alleen verbinding met deze computer** de eigen verbindingen van de app tot deze computer.

- Aanbieders en MCP-servers elders worden onbereikbaar. Ollama, LM Studio en servers op localhost blijven werken.
- Elke geblokkeerde poging staat op dezelfde pagina.
- Docker-containers die worden gemaakt terwijl het aan staat, hebben geen netwerk.

> Programma's die de app start, zoals opdrachten, externe agents en stdio-MCP-servers, vallen er niet onder. Die draaien met het eigen gebruikersaccount. Daarvoor is een Docker-omgeving de aangewezen keuze.

## Back-ups, exports en rapporten

- **Back-up van de configuratie maken…** in Instellingen → Gegevens schrijft instellingen, aanbieders, profielen, omgevingen, regels, MCP-servers, token-optimalisatoren en agent-voorinstellingen naar één bestand. Geheimen, transcripten en vaardigheden zitten er niet in.
- **Terugzetten uit een back-up…** laat eerst zien wat het zou doen. Alles wat opdrachten kan uitvoeren of het beleid kan versoepelen, wacht op goedkeuring.
- **Exporteren als Markdown…** en **Exporteren als JSON…** in het menu van een sessie schrijven het gesprek naar een zelfgekozen bestand, alleen op verzoek.
- **Rapport kopiëren**, onder **Probleem melden** in Instellingen → Gegevens, kopieert een korte diagnose naar het klembord, zonder bekende geheimen en zonder het pad van de thuismap. Er wordt niets verstuurd.

> Een back-up bevat waarden die als platte tekst in configuratiebestanden zijn getypt, dus eerst nakijken voordat hij wordt gedeeld. Een export kan alles bevatten wat de agent heeft gelezen, dus eerst lezen voordat hij wordt doorgestuurd. Wat een rapport bevat, staat op de pagina Problemen oplossen.
