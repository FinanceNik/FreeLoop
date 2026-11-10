# Problemen oplossen

Oplossingen voor een app die niet opent, een agent of aanbieder die faalt en een sessie die vastzit, en hoe een probleem te melden.

## De app opent niet

{{Freeloop-releases zijn niet ondertekend of genotariseerd, dus macOS blokkeert een gedownloade kopie de eerste keer. Eerst proberen de app te openen, dan in Systeeminstellingen → Privacy en beveiliging **Open toch** kiezen. Een zelf gebouwde app opent gewoon.

De app heeft een Mac met Apple Silicon en macOS 14 of later nodig. Na elke update vraagt de sleutelhanger opnieuw om toegang tot de sleutels die de app heeft opgeslagen. Dat is te verwachten: de handtekening verandert bij elke build.|Freeloop-releases zijn niet ondertekend, dus Windows SmartScreen kan een gedownloade kopie de eerste keer tegenhouden. **Meer informatie** en daarna **Toch uitvoeren** kiezen. Een zelf gebouwde app opent gewoon.

De app heeft Windows 10 versie 1809 of later nodig met de Microsoft Edge WebView2 Runtime, die in Windows 11 zit en die het installatieprogramma anders aanbiedt. Opdrachten, hooks en de terminal draaien via Git Bash: meldt een opdracht dat die ontbreekt, dan Git for Windows van git-scm.com installeren en Freeloop opnieuw starten.|De app heeft WebKitGTK en GTK van de distributie nodig. Start hij niet, dan laat starten vanuit een terminal zien wat ontbreekt; daarna het WebKitGTK-pakket van de distributie installeren.

Sleutels gaan via de Secret Service naar de sleutelbos van het systeem, zoals GNOME Keyring of KWallet, dus om een sleutel op te slaan moet er een draaien.}}

## Een agent start niet

Instellingen → Aanbieders openen. Onder **Aangemelde agents** toont elke externe agent **gereed** of de reden waarom hij niet kan starten.

- „Is not on your login PATH” betekent dat de opdracht van de agent niet is geïnstalleerd of dat de login-shell hem niet vindt. De agent installeren, en voor Node.js-agents eerst Node.js.
- Een externe agent meldt zich zelf aan, en Freeloop ziet die aanmelding nooit. De agent één keer in een terminal starten en daar aanmelden, voor Claude Code bijvoorbeeld met `claude`.
- Een met de hand bewerkte agent-voorinstelling start pas nadat die is goedgekeurd in Instellingen → Toestemmingen, onder **Wacht op goedkeuring**.

Start de omgeving van een sessie niet, dan ook die controleren:

- Docker moet draaien, en een mislukte **Statuscontrole** blokkeert de sessie.
- SSH weigert een onbekende hostsleutel. Eén keer in een terminal met `ssh` naar de host verbinden en de vingerafdruk controleren.
- Een omgeving met sandbox voert geen enkele opdracht uit als de sandbox niet kan worden toegepast.

## Een aanbieder geeft een fout

In Instellingen → Aanbieders naast de aanbieder **Verbinding testen** kiezen. Dat toont **Verbonden** met het aantal modellen, of de fout die de aanbieder teruggaf.

- De sleutel, het adres en de modelnaam controleren.
- Bij een lokale server zoals Ollama of LM Studio controleren of die draait.
- Staat **Alleen lokaal** aan in Instellingen → Netwerk, dan zijn aanbieders elders onbereikbaar.

## Een sessie zit vast

Een sessie die niet opschiet, toont een banner die begint met „Komt niet verder” en zegt waarom: een accountlimiet is bereikt (en wanneer die weer vrijkomt), de beurtlimiet is bereikt, hetzelfde hulpprogramma is drie keer achter elkaar mislukt, er waren twee keer terugspoelen in één beurt, of twee beurten achter elkaar eindigden in een fout.

Met de knop **Verdergaan op …** in de banner, of het menu **Ander profiel…**, gaat het werk verder in een nieuwe sessie in hetzelfde project, met een ander profiel, zoals een op een andere aanbieder of agent. Het laatste verzoek van de oude sessie en een samenvatting ervan gaan mee.

> **Verdergaan op …** noemt het profiel dat op de pagina **Profielen** onder **Als een sessie niet verder komt** voor die agent is gekozen. Is een accountlimiet bereikt en stelt het profiel van de sessie `onLimit` in, dan zegt de banner dat het na het einde van de beurt op dat profiel verdergaat, en dat gebeurt zonder te vragen.

## De sandbox start niet op Linux

Op Linux voert **Local, sandboxed** opdrachten uit onder bubblewrap. Het pakket `bubblewrap` installeren als de sessie meldt dat het ontbreekt.

Bubblewrap heeft user namespaces zonder privileges nodig. Ubuntu 24.04 en later onthoudt die via AppArmor aan programma's zonder eigen profiel, dus meldt de sessie dat de sandbox niet zou starten. Om bubblewrap een profiel te geven, dit opslaan als `/etc/apparmor.d/bwrap`:

```
abi <abi/4.0>,
include <tunables/global>

profile bwrap /usr/bin/bwrap flags=(unconfined) {
  userns,
  include if exists <local/bwrap>
}
```

Daarna het profiel laden met `sudo apparmor_parser -r /etc/apparmor.d/bwrap` en de sessie opnieuw starten.

> Hiermee mag elk programma dat bubblewrap uitvoert user namespaces aanmaken, zoals Flatpak ook doet; Ubuntu beperkt ze om sommige aanvallen op de kernel moeilijker te maken. Wie dat liever niet wil, kan opdrachten in plaats daarvan met een Docker-omgeving afschermen.

## Configuratieproblemen en logboeken

Kan een instellingenbestand niet worden gelezen, dan verschijnt een banner: „Problemen in de configuratie; waar nodig gelden standaardwaarden.” Elke regel noemt het bestand, de regel en het veld. De fout herstellen en opslaan. Freeloop neemt wijzigingen binnen een seconde over, en de banner verdwijnt zodra het probleem weg is. Wat op goedkeuring wacht, telt niet als probleem.

Het logboek van de app staat in {{`~/Library/Application Support/freeloop/logs/freeloop.log`|`%AppData%\freeloop\logs\freeloop.log`|`~/.config/freeloop/logs/freeloop.log`}}. Het blijft op deze computer. In Instellingen → Gegevens opent **Logboeken tonen {{in de Finder|in Verkenner|in de bestandsbeheerder}}** de map.

## Een probleem melden

In Instellingen → Gegevens, onder **Probleem melden**, zet **Rapport kopiëren** een korte diagnose op het klembord. **Tonen wat wordt gekopieerd** laat die eerst zien.

Het rapport bevat de versie van Freeloop, de systeemversie, het model, de chip en het geheugen van deze computer, de agent, aanbieder, het model, de modus en de status van de open sessie (of de laatst geopende sessie), en de laatste 200 regels van het logboek. Gesprekken, bestanden, sessietitels en sleutels blijven erbuiten. Geheimen die de app kent, worden verwijderd, en het pad van de thuismap wordt geschreven als `~`.

> Logregels kunnen nog steeds projecten noemen, dus het rapport eerst doorlezen en dan pas delen.

Er wordt nergens iets naartoe gestuurd. Het rapport wordt met de hand in een issue op de GitHub-pagina van het project geplakt. Voor een beveiligingsprobleem daar een privé security advisory gebruiken in plaats van een openbaar issue.
