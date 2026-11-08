import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import App from "./App";
import { setLanguage } from "./i18n";
import "./styles.css";

setLanguage(localStorage.getItem("pc.lang") ?? "auto");
const theme = localStorage.getItem("pc.theme");
if (theme === "light" || theme === "dark") document.documentElement.dataset.theme = theme;

if ("serviceWorker" in navigator) {
  navigator.serviceWorker.register("/sw.js").catch(() => {});
}

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <App />
  </StrictMode>,
);
