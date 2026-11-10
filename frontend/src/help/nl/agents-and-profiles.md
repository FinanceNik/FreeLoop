# Agents en profielen

Kiezen welke agent een sessie uitvoert, erbij aanmelden en de instelling als profiel bewaren.

## Ingebouwde en externe agents

| Agent | Wat het is |
| --- | --- |
| Ingebouwd | De eigen agent van Freeloop. Hij draait op een toegevoegde aanbieder en gebruikt de hulpprogramma's van Freeloop: bestanden lezen en bewerken, opdrachten uitvoeren en zoeken. |
| Extern | Een apart programma dat Freeloop start, zoals Claude Code, Codex CLI, Copilot CLI, Gemini CLI, OpenCode of goose. Het brengt eigen modeltoegang mee en gebruikt de ingestelde aanbieders dus niet. |

Freeloop geeft een externe agent nooit de sleutels van de aanbieders of een aanmelding bij de leverancier. Als een agent niet kan starten, zegt het formulier voor een nieuwe sessie waarom.

> Bij de ingebouwde agent beslist het beleid over elke bestandswijziging en elke opdracht. Een externe agent voert zijn eigen hulpprogramma's uit in zijn eigen proces, en alleen waar hij naar vraagt bereikt Freeloop; voor de rest gelden de weigerregels niet. Voor een agent die niet wordt vertrouwd is een Docker-omgeving de aangewezen keuze.

## Aanmelden bij een externe agent

Een agent met een abonnement, zoals Claude Code, meldt zich aan via zijn eigen programma, in een terminal. Freeloop start het ongewijzigde programma en leest, bewaart of stuurt die aanmelding nooit door.

**Instellingen → Aanbieders** en het formulier voor een nieuwe sessie tonen wat elke agent nodig heeft:

| Agent | Wat hij nodig heeft |
| --- | --- |
| Claude Code | Eén keer `claude` uitvoeren en aanmelden. |
| Codex CLI | `codex login` uitvoeren, of `CODEX_API_KEY` of `OPENAI_API_KEY` toevoegen als geheim van de omgeving van de sessie. |
| Copilot CLI | Een Copilot-abonnement, een eigen of een dat een organisatie verstrekt. Eén keer `npx @github/copilot login` uitvoeren. Elk bericht verbruikt premium requests van het abonnement, en het Copilot-beleid van de organisatie moet Copilot CLI toestaan. In een project dat niet wordt vertrouwd start het niet zolang het project hooks ervoor heeft (`.github/hooks`, of hooks in `.github/copilot/settings.json` of `.claude/settings.json`), omdat het die zonder te vragen zou uitvoeren. |
| OpenCode | `opencode auth login` uitvoeren, of de API-key ervan toevoegen als geheim van de omgeving. Zonder een van beide gebruikt het de gehoste gratis modellen van OpenCode, zodat prompts en code daarheen gaan. |
| goose | Installeren en daarna `goose configure` uitvoeren. |
| Gemini CLI | Een API-key als `GEMINI_API_KEY` toevoegen aan de geheimen van de omgeving. Er geen Google-aanmelding mee gebruiken. |

Daarna de agent in de startgids kiezen met zijn knop, bijvoorbeeld **Claude Code gebruiken**; dat maakt hem ook de standaard voor nieuwe sessies. Of bij het starten van een sessie zijn profiel kiezen.

## Een profiel maken

Een profiel is een bewaarde keuze voor hoe een sessie draait, opgeslagen als YAML-bestand in de map `profiles/` van de gegevensmap van de app. Het ingebouwde profiel Default is de ingebouwde agent met alle hulpprogramma's.

Via **Instellingen → Profielen** voor **Nieuw profiel** kiezen, of voor **Bewerken** bij een bestaand profiel. In te stellen zijn:

- Agent: de ingebouwde of een externe.
- Aanbieder, Model en een reservemodel, voor de ingebouwde agent.
- Toestemmingsmodus, Inspanning, Token-optimalisator en Maximaal aantal modelaanroepen per bericht.
- Systeemprompt, toegevoegd aan die van de ingebouwde agent.
- Aan te bieden hulpprogramma's, vaardigheden en MCP-servers.
- Of de `AGENTS.md` of `CLAUDE.md` van het project wordt gelezen.
- Slash-opdrachten: een naam en een voorbereide prompt.

Regels en hooks staan in de YAML. Om die te bewerken **YAML** kiezen.

> Een hook kan een aanroep van een hulpprogramma tegenhouden. Goedkeuren kan hij er geen.

## Profielen kiezen en wisselen

- Nieuwe sessies gebruiken het standaardprofiel, tenzij in het formulier voor een nieuwe sessie een ander wordt gekozen.
- De standaard is te wijzigen in **Instellingen → Sessies**, bij Profiel.
- Om de ingebouwde standaard te vervangen: bij Default voor Bewerken kiezen en opslaan. **Herstellen** brengt de ingebouwde terug.

Een sessie houdt haar agent. Om verder te gaan met een andere: **Meer acties** openen en onder **Verdergaan met** een profiel kiezen. Freeloop start daarop een nieuwe sessie en vertelt haar wat er in de oude gebeurde; de oude blijft zoals ze was.

Als een sessie haar account- of beurtlimiet bereikt, of steeds mislukt, biedt een banner aan om verder te gaan op een ander profiel.

## Projectprofielen

Een vertrouwd project kan profielen toevoegen in zijn map `.freeloop/profiles/`. Die verschijnen in het formulier voor een nieuwe sessie in een eigen groep.

> Een project kan wat is toegestaan alleen verscherpen. De profielen ervan gebruiken de ingebouwde agent, starten in Vragen of Plannen, voegen alleen weigerregels toe en kunnen geen MCP-servers, geen token-optimalisator en geen reservemodel toevoegen.

Voor een vertrouwd project leest de ingebouwde agent eerst `AGENTS.md`, en `CLAUDE.md` als er geen `AGENTS.md` is. Freeloop schrijft er nooit in, en een profiel kan dit uitzetten.
