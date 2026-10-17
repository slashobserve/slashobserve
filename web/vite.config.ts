import { resolve } from "node:path"
import tailwindcss from "@tailwindcss/vite"
import { tanstackRouter } from "@tanstack/router-plugin/vite"
import react from "@vitejs/plugin-react"
import { defineConfig } from "vite"

const devServer = "http://localhost:8080"

export default defineConfig({
  plugins: [
    tanstackRouter({ target: "react", autoCodeSplitting: true }),
    react(),
    tailwindcss(),
  ],
  resolve: {
    alias: { "@": resolve(import.meta.dirname, "./src") },
  },
  server: {
    proxy: {
      "/api": devServer,
      "/healthz": devServer,
      "/readyz": devServer,
    },
  },
})
