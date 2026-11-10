# Sessies en panelen

In een sessie werken, er veel op orde houden en meerdere naast elkaar zien.

## Een sessie starten

**Nieuwe sessie** kiezen in de zijbalk, of op ⌘K drukken en het daar kiezen. In het berichtvak:

- Enter verstuurt, Shift+Enter voegt een regel toe.
- `@` voegt een bestand uit het project bij.
- `/` toont opdrachten: de modi en wat de agent aanbiedt.
- Bestanden erop neerzetten of plakken om ze bij te voegen.

Terwijl een beurt loopt, wacht een nieuw bericht in een wachtrij tot de beurt eindigt. **Stoppen** beëindigt de beurt.

## Goedkeuringen beantwoorden

Wil de agent iets doen waarover het beleid niet beslist, dan verschijnt een kaart met de opdracht of de bestanden. **Eén keer toestaan**, **Altijd toestaan** of **Weigeren** kiezen, eventueel met een notitie voor de agent.

> **Altijd toestaan** bewaart een regel. Het menu ernaast bepaalt of die voor deze sessie of overal geldt.

## Teruggaan of aftakken

Met de muis naar een van de eigen berichten wijzen:

- **Tot hier terugspoelen** zet de bestanden en het gesprek terug naar vóór dat bericht, en het bericht weer in het vak. Eerst komt er een vraag.
- **Hier aftakken** start een nieuwe sessie met het gesprek tot aan dat bericht.

Is de context voor 80% vol, dan vat **Context comprimeren** in de kop het gesprek samen, zodat elk bericht minder verstuurt.

## Sessies gescheiden houden met worktrees

**In een eigen Git-worktree en branch werken**, in het formulier voor een nieuwe sessie, geeft de sessie een eigen checkout, zodat sessies in één project niet dezelfde bestanden bewerken. Daarvoor moet het project vertrouwd zijn.

Om het werk terug te halen: het committen en daarna **Samenvoegen…** kiezen op het tabblad Diff. Alleen gecommit werk wordt samengevoegd.

## Ordenen

- **Naam wijzigen, taggen, archiveren, verwijderen:** in het ⋯-menu van de sessie, of met een rechtsklik op de sessie in de zijbalk.
- **Zoeken:** de filterknop naast **Sessie zoeken** groepeert en filtert op status, tag en gearchiveerd. **Filter bewaren…** bewaart een filter.

> Verwijderen haalt het transcript van de schijf. Dat kan niet ongedaan worden gemaakt.

## Meerdere tegelijk

| Weergave | Waarvoor |
| --- | --- |
| Panelen | De knoppen onderaan de zijbalk tonen één tot vier sessies. Een sessie erin slepen, of er met ⌘-klik op klikken. |
| Bord | Eén kaart per issue uit de backlog van een werkstroom, een rij per project: Backlog, In behandeling, In beoordeling, Klaar. |
| Wachtrij | Taken die als sessie starten zodra er ruimte is. Het pictogram in de zijbalk toont hoeveel er wachten. |
| Vraagt aandacht (⌘0) | Alles wat op een antwoord wacht, over alle projecten heen. J en K gaan van verzoek naar verzoek. |

## Het rechterpaneel

**Paneel** in de kop van de sessie toont het.

| Tabblad | Toont |
| --- | --- |
| Goedkeuringen | Wat op een antwoord wacht, de regels van de sessie, wat er is besloten |
| Diff | Wijzigingen sinds de start van de sessie; stagen, committen, samenvoegen |
| Bestanden | Het project, met de wijzigingen van de agent gemarkeerd |
| Terminal | Een shell in de omgeving van de sessie |
| Voorbeeld | De draaiende app, per poort |
| Info | Model, modus, tokens, kosten en wat het model ziet |

> De terminal draait met de rechten van het eigen gebruikersaccount, zonder goedkeuringen, en de agent ziet niet wat daar wordt getypt.
