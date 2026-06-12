import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { registerSW } from "virtual:pwa-register";
import { App } from "./app/app";
import "./styles.css";

// autoUpdate mode: a new service worker activates silently; `immediate` makes
// the very first registration happen on load instead of after idle.
registerSW({ immediate: true });

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <App />
  </StrictMode>,
);
