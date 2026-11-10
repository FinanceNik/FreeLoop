# Aan de slag

Van een verse installatie naar de eerste sessie.

## De demo proberen

Op het eerste scherm **Uitproberen met een demo** kiezen, of op ⌘K drukken en **Demo proberen** kiezen. Er is geen sleutel en geen netwerk voor nodig.

- Ze maakt een wegwerpproject en start daarin een sessie met een agent met een vast script.
- De agent probeert dingen die het beleid toestaat, navraagt en weigert, en de goedkeuringsvragen worden beantwoord zoals in een echte sessie.
- **Demo verwijderen** in de banner van de sessie verwijdert de demosessie, de wegwerpmap en de projectvermelding, en verder niets.

## De startgids volgen

**Welkom bij Freeloop** leidt de eerste keer dat de app opent door vier stappen.

- Kiezen hoe agents draaien: een agent waarbij al is aangemeld kiezen met zijn knop, bijvoorbeeld **Claude Code gebruiken**, of **Een API-sleutel of een lokaal model gebruiken** openen en een aanbieder toevoegen.
- Importeren uit Claude Code (optioneel): neemt de MCP-servers, toestemmingsregels en vaardigheden ervan over.
- Een project kiezen met **Kiezen…**. Het vakje **Dit project vertrouwen** staat standaard uit.
- Een sessie starten: het formulier opent met de gekozen map al ingevuld.

**Startgids overslaan** beëindigt de gids. Om hem opnieuw te zien: op ⌘K drukken en **Startgids** kiezen.

## Een aanbieder toevoegen

Een aanbieder is een modeldienst waarmee de ingebouwde agent praat. Voor een agent waarbij al is aangemeld is er geen nodig.

- **Instellingen → Aanbieders** openen en **Aanbieder toevoegen** kiezen.
- De Soort kiezen, Naam en Basis-URL controleren en de API-sleutel plakken. Lokale servers hebben er geen nodig.
- Eventueel een Standaardmodel instellen en dan **Opslaan** kiezen.

Bij het opslaan wordt de verbinding getest en worden de modellen opgesomd. Sleutels gaan naar {{de macOS-sleutelhanger|Windows Referentiebeheer|de sleutelbos van het systeem}}, niet naar een bestand. Meer daarover onder „Aanbieders en modellen”.

## De eerste sessie starten

In de zijbalk **Nieuwe sessie** kiezen, of op ⌘K drukken en het daar kiezen.

- Het Project kiezen, of met Kiezen… een map aanwijzen.
- Profiel op de standaard laten staan. Voor de ingebouwde agent een Aanbieder en een Model kiezen.
- Toestemmingsmodus op de standaard laten staan. Tenzij het profiel of de omgeving iets anders bepaalt, is dat Vragen: de agent vraagt dus voordat hij bestanden wijzigt of opdrachten uitvoert.
- **Sessie starten** kiezen en dan in het berichtvak een taak beschrijven.

## Een project vertrouwen

Een projectmap is niet vertrouwd tot anders wordt aangegeven, omdat een repository bestanden kan bevatten die een agent sturen. Tot dan gebruiken sessies erin alleen de globale instellingen. Freeloop leest de `AGENTS.md` of `CLAUDE.md` en de `.freeloop/`-instellingen ervan niet, voert de hooks ervan niet uit en voert er geen `git status` in uit.

- In het formulier voor een nieuwe sessie somt **Deze map vertrouwen?** de hook-opdrachten op die vertrouwen zou toestaan. **Dit project vertrouwen** kiezen, of zonder vertrouwen starten.
- In een lopende sessie **Project vertrouwen** in de banner kiezen.
- Om het vertrouwen in te trekken: **Dit project niet meer vertrouwen…** kiezen in **Meer acties** (**⋯** in de kopregel) of in het opdrachtenpalet.

Werken in een eigen worktree, de omzeilmodus, starten vanuit een GitHub-issue en profielen vergelijken vereisen vertrouwen.

> Vertrouwen zet goedkeuringen niet uit: de agent vraagt nog steeds, weigerregels gaan nog steeds voor, en de `.freeloop/`-bestanden van een project kunnen toestemmingen verscherpen maar nooit versoepelen. Vertrouwen laat wel de hooks van het project draaien, dus alleen mappen vertrouwen waarvan de inhoud bekend is.

## Verder lezen

- Sessies en panelen: het berichtvak en meerdere sessies tegelijk.
- Agents en profielen: externe agents en profielen.
- Toestemmingen en het beleid: hoe goedkeuringen en regels beslissen.
