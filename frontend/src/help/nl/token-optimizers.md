# Token-optimalisatoren

Shell-uitvoer inkorten voordat het model die ziet, zonder de waarborgen op te geven.

## Wat het doet

De uitvoer van een shell-opdracht gaat terug naar het model en kost tokens. Een token-optimalisator, zoals RTK, is een klein programma dat die uitvoer eerst inkort.

Kortere uitvoer kan iets weglaten wat de agent nodig heeft, zoals een commitbericht of het einde van een grote diff, dus staan optimalisatoren uit tot er een wordt gekozen. Ze werken met de ingebouwde agent en Claude Code, maar niet met andere externe agents, SSH-omgevingen of Claude Code in een Docker-omgeving.

## Er een aanzetten

**Instellingen → Token-optimalisatoren** openen. Er kan op vier plekken worden gekozen:

- **Voor nieuwe sessies** bepaalt de standaard. **Uit** is de beginkeuze.
- Een profiel heeft een eigen veld **Token-optimalisator**.
- Het formulier voor een nieuwe sessie heeft een lijst waarvan het eerste item het profiel of de standaard volgt.
- Een open sessie heeft een lijst in de kop, of **Token-optimalisator: ...** in het opdrachtenpalet.

Een optimalisator die niet is geïnstalleerd, toont **(niet geïnstalleerd)**.

## RTK installeren

Als RTK in het PATH staat, vindt Freeloop het. Anders toont **RTK installeren** vóór de klik de versie, de licentie en de bron (de GitHub-releases van RTK).

- De download is vastgezet op één versie en wordt gecontroleerd tegen een vaste checksum.
- Hij start alleen na een klik, en **Alleen lokaal** in **Instellingen → Netwerk** weigert hem.
- De kopie blijft in de eigen map van Freeloop. **Kopie van Freeloop verwijderen** verwijdert haar.

Voor een zelf geïnstalleerd programma toont de pagina een installatieopdracht met een knop **Kopiëren**. **Controleren** voert het programma één keer uit en meldt of het werkt.

## Een eigen toevoegen

Een bestand in de map `optimizers` van de gegevensmap zetten. Freeloop toont het als **(niet goedgekeurd)** en laat zien wat het voor elke opdracht zou uitvoeren. Alleen op **Goedkeuren** klikken als het te vertrouwen is.

Hetzelfde geldt voor een keuze die uit een bestand komt. Als een configuratie- of profielbestand een optimalisator noemt, starten nieuwe sessies met die optimalisator uit tot hij is goedgekeurd in **Instellingen → Toestemmingen**. Het profiel van een project kan er geen aanzetten.

## Wat veilig blijft

- Goedkeuring komt eerst. De toestemmingsbroker beoordeelt de opdracht zoals het model hem schreef, en pas daarna wordt de optimalisator om een kortere vorm gevraagd.
- Alleen eenvoudige opdrachten worden herschreven. Een pipe, omleiding of substitutie draait zoals geschreven.
- Als de optimalisator iets anders teruggeeft dan dezelfde opdracht in zijn eigen vorm, of ontbreekt, traag is of mislukt, draait het origineel.
- Het tonen van de lijst voert niets uit. Freeloop voert het programma pas uit nadat het is gekozen of gecontroleerd, en weigert er een die in de projectmap is gevonden.

> Een optimalisator kiezen betekent nog steeds dat programma vertrouwen. Het draait met de rechten van de sessie.

> Als RTK een eigen hook heeft in het instellingenbestand van Claude Code, herschrijft die hook de opdrachten van Claude Code, wat hier ook is gekozen, en kan hij opdrachten goedkeuren zonder te vragen. Freeloop verandert dat bestand niet; een sessie toont een waarschuwing als ze zo'n hook vindt.
