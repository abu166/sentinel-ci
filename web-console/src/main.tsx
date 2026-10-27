import React from "react";
import ReactDOM from "react-dom/client";
import App from "./App";
import "@fontsource/inter/latin-400.css";
import "@fontsource/inter/latin-500.css";
import "@fontsource/inter/latin-600.css";
import "@fontsource/jetbrains-mono/latin-400.css";
import "@fontsource/jetbrains-mono/latin-500.css";
import "./styles.css";
const root = ReactDOM.createRoot(document.getElementById("root")!);
async function mountWorkspace() {
  try {
    await Promise.all([
      document.fonts.load('400 12px "Inter"'),
      document.fonts.load('500 12px "Inter"'),
      document.fonts.load('600 14px "Inter"'),
      document.fonts.load('400 11px "JetBrains Mono"'),
      document.fonts.load('500 11px "JetBrains Mono"'),
    ]);
  } catch {
    /* The application remains usable if a local font asset fails. */
  }
  root.render(
    <React.StrictMode>
      <App />
    </React.StrictMode>,
  );
}
void mountWorkspace();
