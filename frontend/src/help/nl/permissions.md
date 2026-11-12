# Toestemmingen en het beleid

Hoe Freeloop beslist wat een agent mag doen, en hoe dat te veranderen is.

## Hoe een verzoek wordt gecontroleerd

Elke keer dat een bestand wordt gelezen of bewerkt, elke opdracht en elke aanroep van een hulpprogramma gaat naar één toestemmingscontrole, voor de ingebouwde agent en externe agents gelijk. Die kijkt naar beschermde paden, weigerregels, de modus en toestaanregels, in die volgorde, en legt alles wat geen daarvan beslist ter beslissing voor.

Niets wordt goedgekeurd omdat de tijd om is. Elke beslissing wordt vastgelegd in het transcript, met wie haar nam.

> Een externe agent zoals Claude Code voert sommige hulpprogramma's uit in zijn eigen proces. Wat hij doet zonder te vragen, bereikt Freeloop nooit.

## Een modus kiezen

Het modusmenu naast het berichtvak toont de huidige modus.

| Modus | Wat hij doet |
| --- | --- |
| **Vragen** | Lezen binnen het project gaat door. Wijzigingen en opdrachten vragen, tenzij een regel beslist. |
| **Wijzigingen accepteren** | Wijzigingen binnen het project gaan door. Opdrachten vragen nog steeds, net als wijzigingen aan bestanden waar andere programma's later naar handelen, zoals `AGENTS.md` of `.github`. |
| **Plannen** | Alleen lezen. De agent stelt een plan voor ter goedkeuring; al het andere wordt tot dan geweigerd. |
| **Automatisch** | De agent keurt zelf goed. Alleen sommige agents bieden dit, en Freeloop ziet alleen waar ze nog naar vragen. |
| **Omzeilen** | Aanroepen draaien zonder te vragen. Beschermde paden, weigerregels en hook-opdrachten blijven gelden. |

> Nieuwe sessies starten in Vragen, Wijzigingen accepteren of Plannen. Automatisch of Omzeilen wordt binnen een sessie gekozen, en alleen in een vertrouwd project. Omzeilen vereist eerst **Omzeilmodus kiesbaar maken voor een sessie** in Instellingen → Toestemmingen. Een opdracht die een sandbox verlaat, vraagt elke keer, ook bij omzeilen.

## Regels

Een regel noemt een hulpprogramma en optioneel een opdracht of pad, en staat het toe of weigert het. Globale regels komen uit Instellingen → Beleid of van **Altijd: overal** en staan in `rules.yaml`. **Altijd: deze sessie** bewaart een regel die met de sessie eindigt. Ook omgevingen, profielen en vertrouwde projecten kunnen regels meebrengen, maar de regels van een project kunnen alleen weigeren.

- Een weigerregel gaat voor op elk niveau en in elke modus, omzeilen inbegrepen.
- Een toestaanregel heft nooit de planmodus of beschermde paden op.
- Een weigerregel op een pad die `write_file` of `edit_file` noemt, geldt voor beide en voor de wijzigingen van een externe agent, die als `edit_file` worden beoordeeld. Een toestaanregel geldt alleen voor het hulpprogramma dat ze noemt.
- Een shell-regel die eindigt op `*` past op een voorvoegsel en keurt alleen één eenvoudige opdracht goed, nooit een keten, pipe of omleiding.
- Een shell-weigerregel past op de woorden die de shell uitvoert: `git "push"`, `git p\ush`, `noglob git push`, `eval "git push"` en `sh -c 'git push'` zijn allemaal `git push`, en vlagletters gelden als verzameling: een regel op `rm -rf *` stopt ook `rm -fr`, `rm -r -f`, `rm -rfv` en `rm --recursive --force`.
- Een opdracht die zijn programma of subopdracht pas bij het uitvoeren samenstelt, zoals `$a push` of `git "$sub"`, is niet met een weigerregel te vergelijken. Zolang een shell-weigerregel van toepassing kan zijn, keurt geen toestaanregel hem goed (er wordt gevraagd) en weigert de omzeilmodus hem.

Op een goedkeuringskaart bewaart **Altijd toestaan** een regel voor precies die opdracht of dat pad. Als één regel de aanroep niet kan dekken, zoals bij een aanroep op meerdere bestanden, wordt de aanroep één keer toegestaan en niets bewaard. De andere antwoorden zijn **Eén keer toestaan** en **Weigeren**. De sneltoetsen zijn ⌘↩, ⌘⇧↩ en ⌘⇧⌫.

> Een weigerregel kan niet tegenhouden wat een agent uit zichzelf doet in zijn eigen automatische modus, omdat dat Freeloop nooit bereikt.

## Beschermde paden en geheime bestanden

Sommige plekken worden nooit gelezen of beschreven, in geen enkele modus, wat een regel ook zegt. Daaronder vallen `~/.ssh`, `~/.aws`, de {{sleutelhangerbestanden|bestanden met referenties|sleutelbosbestanden}}, browsergegevens, aanmeldbestanden van andere hulpprogramma's en de gegevensmap van Freeloop. Instellingen → Beleid toont ze onder **Beschermde paden**.

In een project:

- Agents schrijven nooit in `.git` of `.freeloop`, en nooit buiten het project.
- Lezen buiten het project vraagt.
- Ook het lezen van een bestand waarvan de naam zegt dat het geheimen bevat, zoals `.env`, vraagt, tenzij een toestaanregel het dekt of de sessie in omzeilmodus is. Een zoekopdracht over een map laat zulke bestanden weg en zegt welke, tenzij je ze noemt in het pad of de include; dat vraagt.

> Deze controles gelden voor de bestandshulpprogramma's. Een goedgekeurde shell-opdracht draait met de rechten van het eigen account, tenzij de omgeving een sandbox heeft.

## Hook-opdrachten

Een hook-opdracht is een shell-opdracht die op een bepaald punt in een sessie draait. In Instellingen → Toestemmingen, onder **Hook-opdrachten**, **Hook-opdracht toevoegen** kiezen, dan **Start van de sessie**, **Voor een aanroep van een hulpprogramma**, **Voor een vraag om toestemming** of **Na een aanroep van een hulpprogramma** selecteren en op **Opslaan** drukken. De standaardtime-out is 30 seconden.

De hook-opdracht krijgt de gebeurtenis als JSON op standaardinvoer, en de afsluitcode beslist:

- 0 gaat door.
- 2 blokkeert, met standaardfout als reden. Een geblokkeerde aanroep wordt geweigerd en een geblokkeerde sessiestart stopt de sessie. „Na een aanroep van een hulpprogramma” draait pas als de aanroep klaar is, dus 2 laat daar alleen het bezwaar zien.
- Al het andere, een time-out of een hook-opdracht die niet kan starten, is een mislukking. Als een bewakende hook-opdracht (Voor een aanroep van een hulpprogramma of Voor een vraag om toestemming) mislukt, vraagt de aanroep om toestemming, ook waar de modus of een toestaanregel hem zou hebben toegestaan. Andere hook-opdrachten melden de mislukking en de sessie gaat door.

> Een hook-opdracht kan een aanroep blokkeren maar nooit goedkeuren. De hook-opdrachten van een project, in `.freeloop/hooks.yaml`, draaien pas als het project wordt vertrouwd, en alleen in de vorm die toen te zien was of sindsdien is goedgekeurd.

## Een beslissing controleren en wachtende items goedkeuren

In Instellingen → Beleid beoordeelt **Zou dit worden toegestaan?** een opdracht, wijziging, leesactie of ophaalactie zonder die uit te voeren. Een plek, een modus en een actie kiezen, de opdracht, het pad of de URL invullen en op **Controleren** drukken om het oordeel en de reden te zien.

Een hook-opdracht, toestaanregel, omgeving, instelling, keuze van token-optimalisator of adres van een MCP-server die buiten de app is binnengekomen, bijvoorbeeld uit een teruggezette back-up of door bewerken met de hand, doet niets tot hij is goedgekeurd onder **Wacht op goedkeuring** in Instellingen → Toestemmingen. De hook-opdrachten van een vertrouwd project wachten opnieuw als ze veranderen. Alleen goedkeuren wat bekend voorkomt. Weigerregels hebben geen goedkeuring nodig en gelden meteen.
