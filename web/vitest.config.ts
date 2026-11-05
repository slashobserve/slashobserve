import { resolve } from "node:path"
import { defineConfig } from "vitest/config"

const sharedSpecDir = resolve(import.meta.dirname, "../spec")

export default defineConfig({
  resolve: {
    alias: { "@": resolve(import.meta.dirname, "./src") },
  },
  server: {
    fs: { allow: [import.meta.dirname, sharedSpecDir] },
  },
  test: {
    environment: "happy-dom",
    include: ["src/**/*.test.{ts,tsx}"],
    setupFiles: ["src/test/setup.ts"],
  },
})
