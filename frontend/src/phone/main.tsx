import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { setLanguage } from "../i18n.ts";
import PhoneApp from "./PhoneApp.tsx";
import { pickLanguage } from "./phone.ts";
import { registerWorker } from "./register.ts";
import "./phone.css";

registerWorker();

// The phone app (GUI-052): the language is the phone's, or the one chosen in it.
const chosen = localStorage.getItem("freeloop.phone.language");
setLanguage(chosen === "en" || chosen === "de" || chosen === "nl" ? chosen : pickLanguage(navigator.languages), navigator.languages);
document.documentElement.lang = chosen ?? pickLanguage(navigator.languages);

createRoot(document.getElementById("root") as HTMLElement).render(
  <StrictMode>
    <PhoneApp />
  </StrictMode>,
);
