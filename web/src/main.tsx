import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { App } from "./App";
import { useStore } from "./state/store";
import { buildShareURL } from "./app/share";
import "./styles/theme.css";

// A tiny console/automation handle. It's a stateless playground, so exposing the
// store is harmless and handy for debugging and smoke tests.
(window as unknown as { tailground: unknown }).tailground = { store: useStore, buildShareURL };

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <App />
  </StrictMode>,
);
