// What the page asks of whatever runs it: opening a link outside it, and copying
// text. The desktop app sets Wails' (main.tsx); the phone app (GUI-052), which has
// no Wails runtime to call, keeps the browser's own.

export interface Host {
  openURL(url: string): Promise<void>;
  copy(text: string): Promise<void>;
}

let host: Host = {
  openURL: async (url) => {
    window.open(url, "_blank", "noopener,noreferrer");
  },
  copy: (text) => navigator.clipboard.writeText(text),
};

export function setHost(h: Host) {
  host = h;
}

export const openURL = (url: string) => host.openURL(url);
export const copyText = (text: string) => host.copy(text);
