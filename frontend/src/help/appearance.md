# Appearance and styles

**Settings → Appearance** sets how the window looks: the theme, the text size and the language, the fonts, the colors, and styles that keep fonts and colors under a name.

## Fonts and colors

One font is for text: the interface and messages. The other is for code: code blocks, tool output, diffs and the terminal. A font that is not on this computer shows the app's own font instead.

Colors are set for the light and the dark appearance apart, and a change shows at once. Freeloop warns when text becomes hard to read. The colors are kept in `colors.json` in the data folder, which can also be edited by hand.

## Styles

A style keeps the two fonts and the colors of both appearances under a name, so you can switch between looks in one step. Theme, text size and language are not part of a style.

- **Save as new…** saves the fonts and colors on screen as a style.
- Choose a style under **Style**, or type "Style" in the command palette, to switch to it.
- After a change to a font or a color, the style shows as changed. **Update style** keeps the change in the style; **Revert** goes back to what the style holds.
- Switching away from fonts and colors that no style holds asks first, since they would be lost.

## Share a style

Each style is a file in the `styles` folder of the data folder, which the button {{Show in Finder|Show in File Explorer|Show in file manager}} opens. To share a style, send its file. To add one, put the file in that folder: it shows in the list within a second.

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

The file's name, in lower-case letters, digits and dashes, is the style's ID. A style holds only font names and colors written as hex or `rgb()`, so a style from someone else cannot load anything from the internet or change anything but the look. A value that is neither is reported in Settings and left out.
