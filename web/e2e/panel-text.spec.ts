import type { APIRequestContext, Page } from "@playwright/test"

import { createDashboard, loadSpec, uniqueTitle } from "./api"
import { expectPanelsReady, openDashboard, panel } from "./dashboards"
import { expect, test } from "./fixtures"

async function openTextDashboard(page: Page, request: APIRequestContext) {
  page.on("dialog", (dialog) => {
    throw new Error(`unexpected dialog: ${dialog.message()}`)
  })
  const spec = loadSpec("panel-text")
  spec.title = uniqueTitle("E2E text panels")
  const { id } = await createDashboard(request, spec)
  await openDashboard(page, id)
  await expectPanelsReady(page)
}

for (const colorScheme of ["light", "dark"] as const) {
  test.describe(colorScheme, () => {
    test.use({ colorScheme })

    test("renders markdown with variables", async ({ page, request }) => {
      await openTextDashboard(page, request)

      const notes = panel(page, "Notes")
      await expect(
        notes.getByRole("heading", { name: "Runbook for prod" })
      ).toBeVisible()
      await expect(
        notes.getByRole("cell", { name: "checkout-0" })
      ).toBeVisible()
      await expect(notes.locator("code", { hasText: "code" })).toBeVisible()
    })

    test("keeps links safe", async ({ page, request }) => {
      await openTextDashboard(page, request)

      const notes = panel(page, "Notes")
      const docs = notes.getByRole("link", { name: /the checkout docs/ })
      await expect(docs).toHaveAttribute("target", "_blank")
      await expect(docs).toHaveAttribute("rel", /noopener/)

      const dashboards = notes.getByRole("link", { name: "the dashboards" })
      await expect(dashboards).not.toHaveAttribute("target", /.*/)

      const bad = notes.getByText("bad link")
      await expect(bad).toBeVisible()
      expect((await bad.getAttribute("href")) ?? "").toBe("")
      await expect(notes.getByRole("link", { name: "bad link" })).toHaveCount(0)
    })

    test("never renders HTML", async ({ page, request }) => {
      await openTextDashboard(page, request)

      const notes = panel(page, "Notes")
      await expect(notes.locator("script")).toHaveCount(0)
      await expect(notes.locator("img")).toHaveCount(0)
    })

    test("shows a placeholder with its reason", async ({ page, request }) => {
      await openTextDashboard(page, request)

      const placeholder = panel(page, "Requests by country")
      await expect(placeholder).toHaveAttribute("data-state", "ready")
      await expect(
        placeholder.getByText(
          "Grafana panel type barchart is not supported yet."
        )
      ).toBeVisible()
      await expect(
        placeholder.getByText("Grafana panel type: barchart")
      ).toBeVisible()
    })
  })
}
