import { defineConfig, devices } from "@playwright/test"

import { authFile } from "./e2e/auth-file"

const baseURL =
  process.env.SLASHOBSERVE_E2E_BASE_URL ?? "http://localhost:18080"

function parseShard(value: string | undefined) {
  if (!value) return null
  const [current, total] = value.split("/").map(Number)
  return { current, total }
}

export default defineConfig({
  testDir: "./e2e",
  outputDir: "./test-results",
  fullyParallel: false,
  forbidOnly: Boolean(process.env.CI),
  failOnFlakyTests: Boolean(process.env.CI),
  retries: process.env.CI ? 1 : 0,
  workers: process.env.CI ? 2 : 1,
  shard: parseShard(process.env.PW_SHARD),
  reporter: process.env.CI
    ? [
        ["github"],
        ["html", { open: "never", outputFolder: "playwright-report" }],
      ]
    : "list",
  use: {
    baseURL,
    screenshot: "only-on-failure",
    trace: "on-first-retry",
    video: "retain-on-failure",
  },
  projects: [
    { name: "setup", testMatch: /.*\.setup\.ts/ },
    {
      name: "chromium",
      use: { ...devices["Desktop Chrome"], storageState: authFile },
      dependencies: ["setup"],
    },
  ],
})
