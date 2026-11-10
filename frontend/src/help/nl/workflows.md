# Werkstromen

In fasen werken: het ontwerp vastleggen, opschrijven, in plakjes snijden en de onbewaakte delen aan agents overlaten.

## Wat een werkstroom is

Een werkstroom geeft een sessie slash-opdrachten, één per fase. De ingebouwde, **Plan, slice, run AFK**, volgt de manier van werken met agents van Matt Pocock:

| Opdracht | Wat er gebeurt |
| --- | --- |
| `/grill <brief>` | De agent stelt vragen over de briefing, één tegelijk, elk met het antwoord dat hij voorstelt. In de modus Plannen, dus er wordt niets geschreven. |
| `/prd` | Hij schrijft het vastgelegde ontwerp als PRD in `backlog/prds/`. |
| `/slice` | Hij splitst de PRD op in issues in `backlog/issues/`: dunne plakjes door elke laag, elk gemarkeerd als AFK of mens, met wat ze blokkeert. Daarna commit hij ze. |
| `/afk` | Opent het dialoogvenster van de AFK-run. |
| `/review` | Een tweede agent beoordeelt de wijzigingen van deze sessie, alleen lezend. |
| `/cleanup` | Haalt afgeronde issues en PRD's uit het project. |

Elke tool-aanroep gaat nog steeds door het beleid, in elke fase.

## Er een starten

1. **Instellingen → Werkstromen** openen en op **Vaardigheden installeren** klikken. De vaardigheden van de fasen gaan naar de eigen vaardighedenopslag van Freeloop, met de herkomst vastgelegd. Er wordt niets naar `~/.claude` of het project geschreven.
2. In een sessie de werkstroom kiezen in de lijst naast de optimalisator, of voor een nieuwe sessie **Werkstroom starten…** kiezen in het palet (⌘K).
3. `/` typen om de fasen te zien.

**Voor nieuwe sessies** op de pagina Werkstromen stelt een standaard in, en een profiel kan een eigen keuze maken. **Uit** wint aan beide kanten.

## De backlog

Issues zijn Markdown-bestanden die de agents schrijven en de app leest, alleen in een vertrouwd project:

```
---
title: Search drafts
status: open        # open or done
type: afk           # afk: an agent may do it alone; human: it needs you
blockedBy: [001-store-a-draft]
---
```

Het **Bord** toont een rij voor elk vertrouwd project met een backlog, met een kaart voor elk issue in vier kolommen: **Backlog**, **In behandeling**, **In beoordeling** en **Klaar**. Een kaart toont de titel, het nummer, of een agent (AFK) of een mens het doet, en in Backlog waarop het wacht. Een issue dat de app niet kan lezen, telt als een voor een mens. Een project met een backlog-map dat nog niet wordt vertrouwd, toont een rij met de knop **Project vertrouwen**; er wordt niets uit de map gelezen tot het project wordt vertrouwd. Is er nergens een backlog, dan meldt het Bord dat en biedt het **Workflow starten…** aan. Een klik op een kaart opent het paneel ervan: de tekst van het issue, het bestand, waarop het wacht en wat het ophoudt, en de sessies die eraan werkten, die elk te openen zijn. Het paneel toont alleen; Escape sluit het. Het Bord toont alleen waar elk issue staat, op basis van het bestand en de run; kaarten worden niet met de hand verplaatst. Gewone sessies staan er niet op: die staan in de zijbalk.

Heeft een agent die aan een issue werkt een antwoord nodig, dan blijft de kaart in **In behandeling** met een duidelijke markering **Wacht op een beslissing**, en staat het verzoek, een goedkeuring of een vraag, op de kaart: het is daar te beantwoorden, of via **Bijbehorende sessie openen**. De kop van het Bord telt zulke issues, en een klik erop gaat naar het eerste.

In **In beoordeling** staat een issue op **Agent beoordeelt** zolang de tweede agent het leest, en op **Klaar voor QA** wanneer de eigen QA aan de beurt is: de kaart toont het begin van het antwoord van de review, en **QA klaar** verplaatst hem naar **Klaar**. Stopt een run bij een issue, dan gaat het terug naar **Backlog** met een rode notitie over de reden, tot de volgende run start.

**Klaar** toont de laatste vijf issues van een project, de meest recent door een run afgeronde eerst; **Toon alle …** toont de rest. Het pijltje naast de naam van een project klapt de rij in, en die blijft ingeklapt, behalve dat een rij met een wachtend issue vanzelf opengaat. Met meerdere projecten toont de lijst in de kop er één of allemaal.

**Afgeronde archiveren** verwijdert afgeronde issues, en PRD's waarvan alle issues zijn afgerond, in een eigen commit, zodat latere agents niet op het verkeerde been worden gezet door verouderde documenten. De Git-geschiedenis bewaart ze: `git log -- backlog/` toont ze en `git show` haalt er een terug.

## De AFK-issues uitvoeren

**AFK-issues uitvoeren…** op het Bord, `/afk` of **AFK-issues uitvoeren: <project>** in het palet opent het dialoogvenster van de run. Dat toont het eerste issue, de profielen voor de issues en hun reviews, en de opdrachten die de run nodig zal hebben: de eigen Git-opdrachten en de tests en controles van het project.

- **Toestaan voor de sessies van deze run** laat die opdrachten zonder vragen draaien, alleen in de sessies van de run. Al het andere vraagt nog steeds en wacht in **Vraagt aandacht** op een antwoord.
- De run doet één issue tegelijk, elk in een nieuwe sessie, in een eigen Git-worktree en branch. De eigen checkout blijft zoals hij is.
- Een issue is klaar als het bestand `status: done` zegt. Dan commit de run wat er over is en beoordeelt een tweede agent het, voordat het volgende issue begint.
- De run stopt als er geen AFK-issue gereed is, bij het maximum, als een budget is bereikt of als een sessie eindigt zonder haar issue als klaar te markeren. **Pauzeren** en **Stoppen** staan op het Bord.

> De backlog moet eerst gecommit zijn: de run start vanaf de laatste commit.

Een planning kan 's nachts een run starten: in **Instellingen → Planningen** onder **Start** voor **Een AFK-run van de backlog** kiezen.

## Het werk QA'en

Elk afgerond issue wacht in **Vraagt aandacht** als **Klaar voor QA**, met het antwoord van de review. De wijziging uitproberen en dan op **QA klaar** klikken. Slagen de issues van de run, dan de branch samenvoegen vanuit het tabblad Diff van een van de sessies.

## Eigen werkstromen toevoegen

Een bestand in de map `workflows` van de gegevensmap zetten. Het draait pas nadat het op de pagina Werkstromen met **Goedkeuren** is goedgekeurd, en elke wijziging van het bestand moet opnieuw worden goedgekeurd. Een werkstroom kan het vragen nooit uitzetten.
