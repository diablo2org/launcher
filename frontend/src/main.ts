import { mount } from "svelte";
import "./app.css";
import App from "./App.svelte";
import { SupportService } from "../bindings/github.com/diablo2org/launcher/internal/app";

// Errors nothing else caught go to the launcher's log, for bug reports.
window.addEventListener("error", (e) => {
  SupportService.LogFrontendError(`${e.message} at ${e.filename}:${e.lineno}:${e.colno}`).catch(() => {});
});
window.addEventListener("unhandledrejection", (e) => {
  const r = e.reason;
  SupportService.LogFrontendError(r instanceof Error ? `${r.message}\n${r.stack ?? ""}` : String(r)).catch(() => {});
});

mount(App, { target: document.getElementById("app")! });
