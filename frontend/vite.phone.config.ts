import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// The phone app (GUI-052), built on its own into dist/phone and served by the
// phone server under /phone/. It has no Wails runtime; its Content Security Policy
// comes as a header from that server (internal/remote).
export default defineConfig({
  root: "phone",
  base: "/phone/",
  // The manifest, the icons and the service worker (issue 026) are copied as they are.
  publicDir: "public",
  plugins: [react()],
  build: {
    outDir: "../dist/phone",
    emptyOutDir: true,
    target: "safari16",
  },
});
