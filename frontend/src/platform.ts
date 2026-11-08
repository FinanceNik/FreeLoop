// The system the app runs on (NFR-XPLAT-003): macOS in WebKit, Windows in WebView2,
// Linux in WebKitGTK. The interface behaves the same on all three, each in its own
// conventions: ⌘ on a Mac is Ctrl elsewhere, ⌥ is Alt, and the Finder is File
// Explorer or the file manager. Everything that checks a modifier or names a key or a
// part of the system asks this module. It has no runtime imports, and setPlatform
// lets a test pretend to be any system.

export type OS = "mac" | "windows" | "linux";

// detectOS reads navigator.platform, and the user agent when the platform says
// nothing useful.
export function detectOS(platform: string, userAgent = ""): OS {
  const p = platform.toLowerCase();
  // Checked first: "darwin" holds "win".
  if (/mac|iphone|ipad|darwin/.test(p)) return "mac";
  if (/^win/.test(p)) return "windows";
  if (/linux|x11|bsd/.test(p)) return "linux";
  const ua = userAgent.toLowerCase();
  if (/macintosh|mac os x/.test(ua)) return "mac";
  if (/windows/.test(ua)) return "windows";
  if (/linux|x11|bsd/.test(ua)) return "linux";
  // A Mac until told otherwise: where the app began.
  return "mac";
}

export let os: OS = "mac";
export let isMac = true;

export function setPlatform(platform: string, userAgent = "") {
  os = detectOS(platform, userAgent);
  isMac = os === "mac";
}

const nav = (globalThis as { navigator?: { platform?: string; userAgent?: string } }).navigator;
setPlatform(nav?.platform ?? "", nav?.userAgent ?? "");

export interface Mods {
  metaKey: boolean;
  ctrlKey: boolean;
  altKey?: boolean;
  shiftKey?: boolean;
}

// primaryKey is ⌘ on a Mac and Ctrl elsewhere: the key app shortcuts are made with.
export function primaryKey(e: Mods): boolean {
  return isMac ? e.metaKey : e.ctrlKey;
}

// otherKey is the modifier left over: Control on a Mac, the Windows or Super key
// elsewhere. App shortcuts never use it.
export function otherKey(e: Mods): boolean {
  return isMac ? e.ctrlKey : e.metaKey;
}

// primaryOnly is the primary key with no other modifier, Shift included.
export function primaryOnly(e: Mods): boolean {
  return primaryKey(e) && !otherKey(e) && !e.altKey && !e.shiftKey;
}

const MAC_SIGNS: Record<string, string> = { Meta: "⌘", Ctrl: "⌃", Alt: "⌥", Shift: "⇧", Enter: "↩", Backspace: "⌫", Escape: "⎋", ArrowUp: "↑", ArrowDown: "↓", ArrowLeft: "←", ArrowRight: "→" };
const PC_NAMES: Record<string, string> = { Meta: "Ctrl", Ctrl: "Ctrl", Alt: "Alt", Shift: "Shift", Super: "Super", Escape: "Esc", ArrowUp: "↑", ArrowDown: "↓", ArrowLeft: "←", ArrowRight: "→" };
const PC_ORDER = ["Ctrl", "Super", "Alt", "Shift"];

// keyLabel writes a binding, such as "Meta+Shift+K", the way the system does: ⌘⇧K on a
// Mac, Ctrl+Shift+K elsewhere. Meta is the primary key, so Ctrl on Windows and Linux.
export function keyLabel(binding: string): string {
  if (!binding.includes("+") || binding === "+") return binding.length === 1 ? binding.toUpperCase() : isMac ? (MAC_SIGNS[binding] ?? binding) : (PC_NAMES[binding] ?? binding);
  // The key can itself be "+".
  const parts = binding.endsWith("++") ? [...binding.slice(0, -2).split("+"), "+"] : binding.split("+");
  const key = parts.pop()!;
  const shownKey = key.length === 1 ? key.toUpperCase() : key;
  if (isMac) return [...parts, shownKey].map((p) => MAC_SIGNS[p] ?? p).join("");
  const mods = [...new Set(parts.map((p) => PC_NAMES[p] ?? p))].sort((a, b) => PC_ORDER.indexOf(a) - PC_ORDER.indexOf(b));
  return [...mods, PC_NAMES[shownKey] ?? shownKey].join("+");
}

// chord writes the primary key with optional Shift and a key, for the fixed shortcuts:
// chord("Enter", true) is ⌘⇧↩ on a Mac and Ctrl+Shift+Enter elsewhere.
export function chord(key: string, shift = false): string {
  return keyLabel(["Meta", ...(shift ? ["Shift"] : []), key].join("+"));
}

// ariaKeys writes a binding for aria-keyshortcuts, which names real keys.
export function ariaKeys(binding: string): string {
  return isMac ? binding.replace("Ctrl", "Control") : binding.replace("Meta", "Control").replace("Ctrl", "Control").replace("Super", "Meta");
}

const GLYPH_NAMES: Record<string, string> = { "⌘": "Ctrl", "⌃": "Ctrl", "⌥": "Alt", "⇧": "Shift" };
const GLYPH_KEYS: Record<string, string> = { "↩": "Enter", "⌫": "Backspace", "⎋": "Esc" };

// forPlatform adapts text written for a Mac, such as a help page, to the system.
// {{mac|other}} and {{mac|windows|linux}} choose words per system, and a shortcut
// written with ⌘ ⌥ ⇧ ⌃ becomes Ctrl+Alt+Shift and the key outside a Mac. ⌘-click is
// Ctrl-click there.
export function forPlatform(text: string): string {
  const chosen = text.replace(/\{\{([^{}]*)\}\}/g, (_, body: string) => {
    const ways = body.split("|");
    if (ways.length === 3) return ways[os === "mac" ? 0 : os === "windows" ? 1 : 2];
    return isMac ? ways[0] : (ways[1] ?? ways[0]);
  });
  if (isMac) return chosen;
  return chosen.replace(/([⌘⌃⌥⇧]+)(-click|[^\s⌘⌃⌥⇧])/g, (_, mods: string, key: string) => {
    const names = [...new Set([...mods].map((m) => GLYPH_NAMES[m]))].sort((a, b) => PC_ORDER.indexOf(a) - PC_ORDER.indexOf(b));
    if (key === "-click") return `${names.join("+")}-click`;
    return [...names, GLYPH_KEYS[key] ?? key].join("+");
  });
}

// terminalClipboard says whether a key press in a terminal is the system's copy or
// paste, left to the browser so the terminal's own copy and paste run instead of a
// control character going to the shell. On a Mac ⌘C and ⌘V never reach the shell.
// On Windows and Linux Ctrl+Shift+C and Ctrl+Shift+V copy and paste, as in their
// terminals; on Windows also Ctrl+V, and Ctrl+C while text is selected, as in
// Windows Terminal. Ctrl+C with nothing selected stays the shell's interrupt.
export function terminalClipboard(e: Mods & { key: string }, hasSelection: boolean): "copy" | "paste" | null {
  if (isMac || !e.ctrlKey || e.altKey || e.metaKey) return null;
  const key = e.key.toLowerCase();
  if (key !== "c" && key !== "v") return null;
  const what = key === "c" ? "copy" : "paste";
  if (e.shiftKey) return what;
  if (os !== "windows") return null;
  return what === "paste" || hasSelection ? what : null;
}

// pick chooses a word per system.
function pick(mac: string, windows: string, linux: string): string {
  return os === "mac" ? mac : os === "windows" ? windows : linux;
}

// systemName is the operating system's name.
export function systemName(): string {
  return pick("macOS", "Windows", "Linux");
}

// fileManager is the app that shows folders, for "Show in …".
export function fileManager(): string {
  return pick("Finder", "File Explorer", "file manager");
}

// inFileManager ends "Show this folder …".
export function inFileManager(): string {
  return pick("in the Finder", "in File Explorer", "in the file manager");
}

// showInFiles is the label of a button that shows a folder or file there.
export function showInFiles(): string {
  return `Show in ${fileManager()}`;
}

// secretStore is where the app keeps keys and secrets, as used in a sentence.
export function secretStore(): string {
  return pick("the macOS Keychain", "Windows Credential Manager", "the system keyring");
}
