// The service worker of the phone app (GUI-052): registered only by the phone
// build, under /phone/. It caches nothing and answers no request; it is there for
// push. The desktop window never registers it.
export const workerPath = "/phone/sw.js";
export const workerScope = "/phone/";

export function registerWorker(): void {
  if (!("serviceWorker" in navigator)) return;
  navigator.serviceWorker.register(workerPath, { scope: workerScope }).catch(() => {});
}
