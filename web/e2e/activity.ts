import { readFileSync } from "node:fs"
import { join } from "node:path"

import type { Page } from "@playwright/test"

import type { Spec } from "./api"

const repoRoot = join(import.meta.dirname, "..", "..")

export const revisionsApi = /\/api\/v1\/dashboards\/[^/]+\/revisions(\?.*)?$/

export function readJson<T>(...segments: string[]): T {
  return JSON.parse(
    readFileSync(join(repoRoot, ...segments), { encoding: "utf8" })
  ) as T
}

export function fixturePrompt(file: string): string {
  return readJson<{ prompt: string }>("e2e", "fixtures", "llm", file).prompt
}

export function agentSpec(): Spec {
  return readJson<{ spec: Spec }>(
    "e2e",
    "fixtures",
    "api",
    "dashboard-valid.json"
  ).spec
}

export function card(page: Page) {
  return page.getByRole("complementary", { name: "Activity" })
}

export function entries(page: Page) {
  return card(page)
    .getByRole("list", { name: "Activity" })
    .locator(":scope > li")
}

export function entryWith(page: Page, text: string) {
  return entries(page).filter({ hasText: text })
}

export async function holdRequestsRunning(page: Page) {
  let held = true
  await page.route(revisionsApi, async (route) => {
    if (route.request().method() !== "GET" || !held) {
      await route.fallback()
      return
    }
    const response = await route.fetch()
    const body = (await response.json()) as {
      revisions: Record<string, unknown>[]
    }
    body.revisions = body.revisions.map((revision) => ({
      ...revision,
      status: "running",
      attempts: 1,
      error: null,
      result_version: null,
      finished_at: null,
    }))
    await route.fulfill({ response, json: body })
  })
  return () => {
    held = false
  }
}
