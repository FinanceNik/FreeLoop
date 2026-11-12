// The phone app's service worker (GUI-052). It shows a notification when the computer
// sends a Web Push (issue 028) and opens the app when one is tapped. It answers no
// request and keeps nothing. A push holds only a kind and a session's ID, so the text
// comes from here, in the phone's language; it never names the session.
self.addEventListener("install", () => self.skipWaiting());
self.addEventListener("activate", (event) => event.waitUntil(self.clients.claim()));

const text = {
  en: { approval: "A session needs you", done: "A session finished", failed: "A session stopped with an error", budget: "A session reached its budget", other: "Something happened in a session" },
  de: { approval: "Eine Sitzung braucht dich", done: "Eine Sitzung ist fertig", failed: "Eine Sitzung wurde mit einem Fehler beendet", budget: "Eine Sitzung hat ihr Budget erreicht", other: "In einer Sitzung ist etwas passiert" },
  nl: { approval: "Een sessie heeft je nodig", done: "Een sessie is klaar", failed: "Een sessie is met een fout gestopt", budget: "Een sessie heeft haar budget bereikt", other: "Er is iets gebeurd in een sessie" },
};

// notification is what to show for a push's data: a title from its kind and a link
// to its session. A payload that is missing or not what was expected still gives a
// notification, since a browser may refuse a worker that shows none.
function notification(data, language) {
  const words = text[String(language || "en").slice(0, 2).toLowerCase()] || text.en;
  const kind = data && typeof data.kind === "string" && Object.hasOwn(words, data.kind) ? data.kind : "other";
  const id = data && typeof data.sessionId === "string" && /^[A-Za-z0-9_-]{1,64}$/.test(data.sessionId) ? data.sessionId : "";
  return {
    title: "Freeloop",
    options: {
      body: words[kind],
      icon: "/phone/icon-192.png",
      badge: "/phone/icon-192.png",
      tag: id ? kind + ":" + id : kind,
      data: { url: "/phone/" + (id ? "#session=" + id : "") },
    },
  };
}
self.freeloopNotification = notification;

self.addEventListener("push", (event) => {
  let data = null;
  try {
    data = event.data ? event.data.json() : null;
  } catch (e) {
    data = null;
  }
  const n = notification(data, self.navigator && self.navigator.language);
  event.waitUntil(self.registration.showNotification(n.title, n.options));
});

self.addEventListener("notificationclick", (event) => {
  event.notification.close();
  const url = (event.notification.data && event.notification.data.url) || "/phone/";
  event.waitUntil(
    self.clients.matchAll({ type: "window", includeUncontrolled: true }).then((open) => {
      const there = open.find((c) => c.url.includes("/phone/"));
      if (there && there.navigate) return there.navigate(url).then((c) => c && c.focus());
      return self.clients.openWindow(url);
    }),
  );
});
