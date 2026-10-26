import React from "react";
import { createRoot } from "react-dom/client";
import { App } from "./App";
import { DisconnectOverlay } from "./DisconnectOverlay";
import "./style.css";

createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    {new URLSearchParams(location.search).get("view") === "disconnect-progress" ? <DisconnectOverlay /> : <App />}
  </React.StrictMode>,
);
