import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import App from "./App.tsx";
import ErrorBoundary from "./ErrorBoundary.tsx";
import { framed } from "./preview.ts";
import "./app.css";
import { Browser, Clipboard } from "@wailsio/runtime";
import { setHost } from "./host.ts";

// Links open in the user's browser, and copying goes through the app (host.ts).
setHost({ openURL: (url) => Browser.OpenURL(url), copy: (text) => Clipboard.SetText(text) });

// A page in the Preview tab can load this page in a frame of its own. Such a
// copy has the app's address, so its engine calls pass the guard, and a page
// laid over an invisible one could turn a click on a harmless button into an
// approval. It never draws.
if (!framed(window)) {
  createRoot(document.getElementById("root") as HTMLElement).render(
    <StrictMode>
      <ErrorBoundary>
        <App />
      </ErrorBoundary>
    </StrictMode>,
  );
}
