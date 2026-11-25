import { resolve } from "node:path"
import tailwindcss from "@tailwindcss/vite"
import { tanstackRouter } from "@tanstack/router-plugin/vite"
import react from "@vitejs/plugin-react"
import { defineConfig } from "vite"

const devServer = "http://localhost:8080"

const gridPackages =
  /node_modules[\\/](ag-grid-community|ag-grid-react|ag-stack)[\\/]/

const vendorPackages =
  /node_modules[\\/](react|react-dom|scheduler|@tanstack|@base-ui|@floating-ui|zustand|lucide-react|openapi-fetch|sonner|cn|class-variance-authority|use-sync-external-store)[\\/]/

export default defineConfig({
  plugins: [
    tanstackRouter({ target: "react", autoCodeSplitting: true }),
    react(),
    tailwindcss(),
  ],
  resolve: {
    alias: { "@": resolve(import.meta.dirname, "./src") },
  },
  build: {
    rolldownOptions: {
      output: {
        codeSplitting: {
          groups: [
            { name: "grid", test: gridPackages, priority: 1 },
            { name: "vendor", test: vendorPackages, priority: 2 },
          ],
        },
      },
    },
  },
  server: {
    proxy: {
      "/api": devServer,
      "/healthz": devServer,
      "/readyz": devServer,
    },
  },
})
