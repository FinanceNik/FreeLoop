# Weergave en stijlen

**Instellingen → Weergave** bepaalt hoe het venster eruitziet: het thema, de tekstgrootte en de taal, de lettertypen, de kleuren, en stijlen die lettertypen en kleuren onder een naam bewaren.

## Lettertypen en kleuren

Eén lettertype is voor tekst: de interface en de berichten. Het andere is voor code: codeblokken, uitvoer van tools, diffs en de terminal. Een lettertype dat niet op deze computer staat, wordt vervangen door het eigen lettertype van de app.

Kleuren worden apart ingesteld voor de lichte en de donkere weergave, en een wijziging is meteen te zien. Freeloop waarschuwt als tekst moeilijk leesbaar wordt. De kleuren staan in `colors.json` in de gegevensmap, die ook met de hand te bewerken is.

## Stijlen

Een stijl bewaart de twee lettertypen en de kleuren van beide weergaven onder een naam, zodat wisselen tussen twee uiterlijken één stap is. Thema, tekstgrootte en taal horen niet bij een stijl.

- **Als nieuwe opslaan…** slaat de lettertypen en kleuren op het scherm op als stijl.
- Een stijl kiezen onder **Stijl**, of "Stijl" typen in het opdrachtenpalet, om ernaar te wisselen.
- Na een wijziging van een lettertype of een kleur staat de stijl als gewijzigd. **Stijl bijwerken** houdt de wijziging in de stijl; **Terugdraaien** gaat terug naar wat de stijl bevat.
- Wegwisselen van lettertypen en kleuren die in geen stijl staan, vraagt eerst, omdat ze anders verloren gaan.

## Een stijl delen

Elke stijl is een bestand in de map `styles` in de gegevensmap, die de knop {{Weergeven in Finder|Weergeven in Verkenner|Weergeven in bestandsbeheerder}} opent. Om een stijl te delen, het bestand versturen. Om er een toe te voegen, het bestand in die map zetten: binnen een seconde staat de stijl in de lijst.

```
{
  "version": 1,
  "name": "Paper",
  "textFont": "Inter",
  "codeFont": "",
  "light": { "accent": "#7c3aed", "bg": "#fdfcf8" },
  "dark": {}
}
```

De naam van het bestand, in kleine letters, cijfers en streepjes, is de ID van de stijl. Een stijl bevat alleen namen van lettertypen en kleuren als hexwaarde of `rgb()`, dus een stijl van iemand anders kan niets van internet laden en niets anders veranderen dan het uiterlijk. Een waarde die geen van beide is, wordt gemeld in de instellingen en weggelaten.
