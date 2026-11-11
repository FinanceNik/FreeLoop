# Spraakinvoer

Een prompt inspreken in plaats van typen. De spraak wordt op deze computer tekst.

## Dicteren

Op de microfoon in het berichtvak klikken, of op ⌘⇧D drukken, en spreken. Een tweede klik of nog eens ⌘⇧D zet de woorden in het berichtvak waar de cursor stond. Er wordt niets verstuurd: de tekst lezen, aanpassen en dan zoals altijd versturen. Escape gooit de opname weg.

- Een opname mag tot 3 minuten duren.
- De eerste keer vraagt {{macOS|Windows|het systeem}} of Freeloop de microfoon mag gebruiken.
- Was er niets te horen, dan wordt er niets toegevoegd. Dan iets luider spreken of de microfoon controleren.

## Het spraakmodel

Spraakinvoer draait Whisper large-v3-turbo met whisper.cpp, allebei onder de MIT-licentie. Het is goed in het Engels, Duits en Nederlands, en in veel andere talen.

Bij het eerste dictaat vraagt Freeloop het model één keer te downloaden: ongeveer 870 MB van Hugging Face, en op Windows en Linux whisper.cpp van GitHub. Elk bestand is vastgezet en wordt gecontroleerd tegen een bekende checksum voordat het wordt bewaard. **Instellingen → Spraakinvoer** laat zien waar het staat en verwijdert het. In de modus Alleen lokaal wordt de download geweigerd.

Hoe snel het gaat, hangt af van de computer. Op een Mac met Apple-chip duurt een zin ongeveer een seconde. Op een computer zonder grafische chip die whisper.cpp kan gebruiken, kan een lange opname enkele seconden duren.

## Welke taal

Standaard laat Freeloop het model weten dat er gesproken wordt in de taal van het venster. In **Instellingen → Spraakinvoer** is een andere taal op te geven, of kan het model hem elke keer herkennen. Hem opgeven is nauwkeuriger: een korte Nederlandse zin wordt gauw voor Duits aangezien.

## Wat op deze computer blijft

- Het gesprokene gaat van de microfoon naar whisper.cpp op deze computer, en wordt nooit naar schijf geschreven of ergens heen gestuurd.
- whisper.cpp draait alleen tijdens het dicteren, en stopt zodra de tekst terug is.

## Als het niet werkt

- **Freeloop mag de microfoon niet gebruiken**: toestaan in {{Systeeminstellingen → Privacy en beveiliging → Microfoon|Instellingen → Privacy en beveiliging → Microfoon, met de toegang voor bureaublad-apps aan|de privacy- of geluidsinstellingen van het systeem}} en het opnieuw proberen.
- {{Na een update vraagt macOS het misschien opnieuw: de versies van Freeloop zijn niet ondertekend met een Apple-certificaat.|Op Windows heeft whisper.cpp het Microsoft Visual C++ Redistributable (2015–2022) nodig; Freeloop zegt het als het ontbreekt.|Op Linux heeft whisper.cpp glibc 2.35 of nieuwer en libgomp1 nodig, en het venster de PipeWire- of PulseAudio-plug-in van GStreamer.}}
