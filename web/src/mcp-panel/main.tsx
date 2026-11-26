import { StrictMode } from "react"
import { createRoot } from "react-dom/client"

import { McpPanel } from "./mcp-panel"
import "./panel.css"

const rootElement = document.getElementById("root")
if (!rootElement) throw new Error("missing #root element")

createRoot(rootElement).render(
  <StrictMode>
    <McpPanel />
  </StrictMode>
)
