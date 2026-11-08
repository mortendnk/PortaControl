// Minimal service worker: makes the web UI installable. Requests always go to
// the network (the state of the air conditioner must never be stale).
self.addEventListener("install", () => self.skipWaiting());
self.addEventListener("activate", (e) => e.waitUntil(self.clients.claim()));
self.addEventListener("fetch", () => {});
