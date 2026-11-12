// The phone app's service worker (GUI-052). It exists so the app can receive Web
// Push once that is built (issue 029). It answers no request and keeps nothing.
self.addEventListener("install", () => self.skipWaiting());
self.addEventListener("activate", (event) => event.waitUntil(self.clients.claim()));
