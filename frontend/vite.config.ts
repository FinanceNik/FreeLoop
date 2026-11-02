import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import wails from "@wailsio/runtime/plugins/vite";

// A backstop for built pages: no script gets in from anywhere but the app, so
// even a rendering bug in what a model wrote has nothing to run. Not in `make
// dev`, whose live reload needs an inline script. The frames are the Preview tab.
const CSP = [
  "default-src 'self'",
  "script-src 'self'",
  "style-src 'self' 'unsafe-inline'",
  "img-src 'self' data: blob:",
  "font-src 'self' data:",
  "connect-src 'self' ws://localhost:* ws://127.0.0.1:* wails:",
  "frame-src http://localhost:* http://127.0.0.1:* http://[::1]:* https://localhost:* https://127.0.0.1:*",
  "object-src 'none'",
  "base-uri 'none'",
  "form-action 'none'",
].join("; ");

const contentSecurityPolicy = {
  name: "freeloop-csp",
  apply: "build" as const,
  transformIndexHtml: (html: string) => html.replace("<head>", `<head>\n    <meta http-equiv="Content-Security-Policy" content="${CSP}" />`),
};

// https://vitejs.dev/config/
export default defineConfig({
  server: {
    host: "127.0.0.1",
    port: Number(process.env.WAILS_VITE_PORT) || 9245,
    strictPort: true,
  },
  plugins: [react(), wails("./bindings"), contentSecurityPolicy],
});
