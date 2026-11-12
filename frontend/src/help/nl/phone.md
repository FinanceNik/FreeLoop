# De telefoon

Sessies onderweg volgen vanaf de telefoon, via het eigen privénetwerk.

## Wat de telefoon kan

- Live zien wat wacht, en elke sessie.
- Een verzoek één keer toestaan of weigeren. Formulieren, plannen, regels en instellingen wachten op de computer.
- Een sessie een bericht sturen, of haar beurt stoppen.

Berichten en beslissingen vanaf de telefoon zeggen dat in de sessie, met de naam van de telefoon.

## Instellen

1. Onder **Instellingen → Telefoon** **De telefoon antwoorden** aanzetten. Freeloop antwoordt dan alleen op een poort van deze computer en blijft draaien als het venster sluit.
2. Tailscale installeren op deze computer en de telefoon, beide aanmelden met hetzelfde account en in het beheer van Tailscale HTTPS-certificaten aanzetten.
3. De opdracht die de pagina toont één keer in een terminal uitvoeren. Die biedt Freeloop alleen aan op het eigen tailnet. De pagina toont ook hoe je hem stopt.
4. **Telefoon koppelen…** kiezen, het getoonde adres op de telefoon openen en op de computer bevestigen.

Op een iPhone geeft **Zet op beginscherm** in Safari een eigen pictogram; vraagt het daar opnieuw om te koppelen, dan nog een keer koppelen.

## Bereikbaar blijven

- {{De Mac moet wakker blijven. **Deze computer wakker houden zolang een sessie werkt of wacht** voorkomt slapen bij inactiviteit; dichtgeklapt slaapt hij toch, tenzij hij aan de stroom en een extern scherm hangt.|De computer moet wakker blijven. **Deze computer wakker houden zolang een sessie werkt of wacht** voorkomt slapen bij inactiviteit; dichtklappen kan hem toch laten slapen.|De computer moet wakker blijven. **Deze computer wakker houden zolang een sessie werkt of wacht** vraagt het systeem niet te slapen bij inactiviteit; dichtklappen volgt de eigen instelling.}}
- Freeloop moet blijven draaien: het venster sluiten verbergt het zolang de telefoon aan staat. Afsluiten stopt het.
- De modus Alleen lokaal zet de telefoon uit, omdat de sessies anders deze computer zouden verlaten.

## Wat privé blijft

- Freeloop antwoordt alleen op deze computer. Tailscale brengt het versleuteld naar de telefoon, en geen ander apparaat bereikt het.
- Alleen gekoppelde en op de computer bevestigde telefoons komen erin. **Alle telefoons afmelden** op de pagina beëindigt ze allemaal, met hun open streams, direct.
- De sleutel van een telefoon staat in de eigen opslag van de telefoon-app en wordt als header verstuurd, nooit als cookie. Een andere pagina onder dezelfde Tailscale-naam krijgt hem dus nooit, en hij werkt niet onder een andere Tailscale-login of na 90 dagen ongebruikt. Een telefoon die met een eerdere build is gekoppeld, moet opnieuw koppelen.
- Een melding gaat via de pushdienst van je telefoon (die van Apple, Google of Mozilla), zo versleuteld dat alleen die telefoon hem leest, en bevat alleen wat er gebeurde en het ID van een sessie. De pushdienst merkt dat er een bericht is verstuurd en wanneer. De Alleen-lokaal-modus verstuurt er geen.
- Opdrachten die agents uitvoeren, bereiken de poort zoals elke op deze computer, maar krijgen daar zonder gekoppelde telefoon niets.
- Tailscale vermeldt HTTPS-certificaten openbaar, dus de Tailscale-naam van de computer wordt openbaar.
