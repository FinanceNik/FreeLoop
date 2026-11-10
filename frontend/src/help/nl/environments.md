# Omgevingen

Kiezen waar de opdrachten van een sessie draaien en wat elke keuze tegenhoudt.

Een omgeving is waarmee de opdrachten van een sessie draaien: variabelen, geheimen, de hulpprogramma's die de agent mag gebruiken en de modus waarin nieuwe sessies starten. Omgevingen worden beheerd in Instellingen → Omgevingen met **Omgeving toevoegen**, **Bewerken** en **Verwijderen** (**Herstellen** voor een ingebouwde). Ingebouwd zijn „Local” en „Local, sandboxed”.

## Een omgeving kiezen

- Voor één sessie het menu **Omgeving** in het formulier voor een nieuwe sessie gebruiken. Daar staat bij of opdrachten op deze computer vragen of in een sandbox draaien.
- Als standaard begint het formulier met de standaardomgeving van het project, of, voor projecten zonder eigen omgeving, met de omgeving die in Instellingen → Sessies is gekozen.
- **Startmodus van nieuwe sessies** bepaalt de modus waarin sessies beginnen. Een profiel met een eigen modus gaat voor.

De stevigste greep op een repository dat niet te vertrouwen is, geeft Docker. Voor een snelle, rustigere sessie met eigen code past Local, sandboxed.

## Local

Opdrachten draaien op deze computer, in de projectmap, met alle rechten van het gebruikersaccount.

> Niets beperkt ze. De toestemmingscontrole vraagt nog steeds voordat een opdracht draait, maar een goedgekeurde opdracht kan alles lezen en wijzigen wat het eigen account kan. Local is geen sandbox.

Standaard kunnen de opdrachten, agents en terminals van een lokale omgeving de eigen SSH-agent gebruiken, zodat `git push` en `git pull` via SSH werken. Dat betekent ook dat een goedgekeurde opdracht de eigen SSH-sleutels voor iets anders kan gebruiken, zoals inloggen op een server.

Om dat te voorkomen, in de omgeving **Opdrachten en agents mogen de eigen SSH-agent gebruiken** uitzetten. Git via SSH heeft dan een andere toegang nodig, zoals HTTPS.

## Local, sandboxed

Opdrachten draaien onder {{de macOS-sandbox|een sandbox, die Windows nog niet heeft: op Windows meldt deze omgeving dat en voert niets uit, dus voor beperkte opdrachten een Docker-omgeving gebruiken|bubblewrap (zie Problemen oplossen als het niet start)}}. Ze kunnen alleen in het project schrijven en daarbuiten weinig lezen, dus de opdrachten van de ingebouwde agent draaien zonder te vragen, en weigerregels gaan nog steeds voor. Voor het verlaten van de sandbox wordt elke keer gevraagd. Als de sandbox niet kan worden toegepast, draait de opdracht niet.

Daarbinnen geldt voor opdrachten:

- schrijven alleen in het project, de caches ervan en tijdelijke bestanden
- geen lezen van de thuismap (behalve gangbare toolchains), opstartbestanden van de shell of de eigen configuratie van git
- geen schrijven in `.git/config`, git-hooks, geneste `.git`- of `.freeloop`-mappen of andere bestanden waar hulpprogramma's later naar handelen
- standaard alleen het netwerk van deze computer zelf bereikbaar

**Netwerk** kan op **Alleen deze computer**, **Deze computer en de hosts die hier staan** of **Geen** worden gezet. Met een lijst van hosts moet een hulpprogramma de proxy gebruiken die de sandbox opzet. Daarnaast kunnen meer mappen worden toegevoegd die gelezen of beschreven mogen worden.

> De sandbox dekt niet de eigen hulpprogramma's van een externe agent, MCP-servers of wat in de terminal van een sessie wordt getypt. Daarbinnen is lezen buiten de thuismap open, wordt `/tmp` gedeeld en bereikt Alleen deze computer nog steeds al het andere dat op deze computer luistert. Een opdracht kan nog steeds een bestand schrijven dat later met de hand wordt uitgevoerd, zoals een `Makefile`.

## Docker

Elke sessie krijgt een eigen container, waarin de projectmap op hetzelfde pad wordt gedeeld. Opdrachten en externe agents draaien daarin, dus ze zien het project en de image, niet de rest van de eigen bestanden.

- Een image opgeven, of de `.devcontainer/devcontainer.json` van het project gebruiken; daarvoor moet het project vertrouwd zijn.
- Een **Inrichtingsopdracht** draait één keer nadat de container is gemaakt, en een **Statuscontrole** draait voor elke start van een sessie.
- Nodig is Docker Desktop, of Colima met de docker-CLI.

> Een container is geen kluis. Hij heeft netwerktoegang, draait als de gebruiker van de image (vaak root) en kan alles in de gedeelde projectmap wijzigen. De uitzonderingen zijn alleen-lezen gekoppeld: de `.freeloop`-map van het project, de devcontainer-bestanden, en `.git/config` en de git-hooks van de repository. De rest van `.git` blijft beschrijfbaar, dus met zo'n checkout in de eigen terminal voorzichtig zijn.

## SSH

Opdrachten draaien op een andere machine, in een werkkopie die gelijk wordt gehouden met het project op deze computer. Bij **Host** een naam uit `~/.ssh/config` of `user@host` invullen.

Eerst één keer in een terminal `ssh` naar die host uitvoeren en de vingerafdruk controleren, want Freeloop accepteert uit zichzelf geen hostsleutel. Inloggen gaat met de eigen sleutels en ssh-agent; Freeloop leest ze nooit.

De andere machine wordt niet vertrouwd:

- Geheimen, bestanden die onder een weigerregel vallen en wat het project negeert, worden er niet naartoe gekopieerd.
- Alleen gewone bestanden komen terug. Instellingen waarnaar deze computer of een agent zou handelen, zoals `.git`, `.claude` en `AGENTS.md`, komen niet terug.
- Andere wijzigingen komen wel terug, dus de diff lezen voordat er iets wordt uitgevoerd.
- De code, en de variabelen en geheimen van deze omgeving, gaan naar die machine.

Alleen een machine gebruiken die gemist kan worden. Voorlopig werkt een SSH-omgeving alleen met de ingebouwde agent.

## Variabelen en geheimen

- **Variabelen** zijn één `KEY=value` per regel en worden toegevoegd aan de omgeving van elke opdracht.
- **Geheimen** worden toegevoegd met **Geheim toevoegen**. Ze worden bewaard in {{de macOS-sleutelhanger|Windows Referentiebeheer|de sleutelbos van het systeem}}, bij de start van een opdracht als variabelen meegegeven en uit transcripten verwijderd.

Het verwijderen dekt waarden die Freeloop kent. Tekst die een model al heeft ontvangen, verandert niet, dus een geheim dat een opdracht afdrukt, kan het model bereiken.

Een omgeving die met de hand in haar YAML-bestand is bewerkt, wacht op goedkeuring in Instellingen → Toestemmingen voordat een sessie haar kan gebruiken.
