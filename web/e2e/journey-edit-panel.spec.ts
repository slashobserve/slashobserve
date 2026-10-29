import type { Page } from "@playwright/test"

import { agentSpec, card, entryWith, fixturePrompt } from "./activity"
import { addVersion, createDashboard, getDashboard, uniqueTitle } from "./api"
import { expectPanelsReady, openDashboard, panel } from "./dashboards"
import { draftPreviewRejectionLogged, expect, test } from "./fixtures"

const selectAll = process.platform === "darwin" ? "Meta+A" : "Control+A"
const agentTimeout = 90_000
const panelId = "rps"
const panelTitle = "Requests per second"
const editedTitle = "Requests per second, edited"
const note = "Added p95 and a warning"
const suggestPrompt = fixturePrompt("suggest-p95.json")
const unclosedQuery = "sum(rate(http_requests_total[5m])"
const closedQuery = `${unclosedQuery})`

function editor(page: Page) {
  return page.getByRole("complementary", { name: `Edit ${panelTitle}` })
}

function queryGroup(page: Page, ref: string) {
  return editor(page).getByRole("group", { name: `Query ${ref}` })
}

function viewSwitch(page: Page) {
  return editor(page).getByRole("radiogroup", { name: "Editor view" })
}

function panelMenu(page: Page, title: string) {
  return page.getByRole("button", { name: `Panel menu: ${title}` })
}

async function openEditor(page: Page, title: string) {
  await panelMenu(page, title).click()
  await page.getByRole("menuitem", { name: "Edit", exact: true }).click()
}

for (const colorScheme of ["light", "dark"] as const) {
  test.describe(colorScheme, () => {
    test.use({ colorScheme, viewport: { width: 1440, height: 1000 } })
    test.setTimeout(240_000)

    test("edits a panel with a preview, a suggestion, settings and JSON, then saves and applies to a newer version", async ({
      page,
      request,
      consoleErrors,
    }) => {
      const spec = agentSpec()
      spec.title = uniqueTitle("E2E journey 3")
      const { id } = await createDashboard(request, spec)

      await test.step("open the editor", async () => {
        await openDashboard(page, id)
        await expectPanelsReady(page)
        await openEditor(page, panelTitle)

        await expect(editor(page)).toBeVisible()
        await expect(page).toHaveURL(/panel=rps&edit=query/)
        await expect(panel(page, panelTitle)).toContainText("Editing")
        await expect
          .poll(async () => (await editor(page).boundingBox())?.width ?? 0)
          .toBeGreaterThanOrEqual(700)
        const box = await editor(page).boundingBox()
        expect(box?.width).toBeLessThanOrEqual(800)
      })

      await test.step("preview", async () => {
        await expect(
          editor(page)
            .getByRole("status")
            .filter({ hasText: /^Draft ran / })
        ).toHaveText(/^Draft ran .+, \d+ series$/, { timeout: 30_000 })
      })

      await test.step("suggest", async () => {
        await editor(page)
          .getByRole("textbox", { name: "Ask the agent to change this query" })
          .fill(suggestPrompt)
        await editor(page).getByRole("button", { name: "Suggest" }).click()

        const suggestion = editor(page).getByRole("region", {
          name: "Agent suggestion",
        })
        await expect(
          suggestion.getByRole("heading", { name: "Proposed change" })
        ).toBeVisible({ timeout: agentTimeout })
        await suggestion.getByRole("button", { name: "Accept" }).click()

        await expect(queryGroup(page, "B")).toBeVisible()
        await expect(
          queryGroup(page, "B").getByText("New", { exact: true })
        ).toBeVisible()
        await expect(
          editor(page).getByText("1 change: new query B")
        ).toBeVisible()
        await expect(editor(page).getByText("Saved in v1")).toBeVisible({
          timeout: 30_000,
        })
      })

      await test.step("lint and fix query A", async () => {
        const queryA = editor(page).getByRole("textbox", {
          name: "Query A",
          exact: true,
        })
        await queryA.click()
        await page.keyboard.press(selectAll)
        await page.keyboard.insertText(unclosedQuery)
        await expect(queryA).toHaveText(unclosedQuery)
        await expect(
          queryGroup(page, "A").locator(".cm-lintRange-error")
        ).not.toHaveCount(0, { timeout: 10_000 })
        await draftPreviewRejectionLogged(consoleErrors)

        await page.keyboard.press("End")
        await page.keyboard.insertText(")")
        await expect(queryA).toHaveText(closedQuery)
        await expect(
          queryGroup(page, "A").locator(".cm-lintRange-error")
        ).toHaveCount(0, { timeout: 10_000 })
        await expect(
          queryGroup(page, "A").getByText("Changed", { exact: true })
        ).toBeVisible()
      })

      await test.step("metrics browser", async () => {
        const toggle = editor(page).getByRole("button", {
          name: "Metrics and labels",
          exact: true,
        })
        await toggle.click()
        const browser = editor(page).getByRole("region", {
          name: "Metrics and labels",
        })
        await browser
          .getByRole("textbox", { name: "Search metrics" })
          .fill("http_requests")
        await expect(
          browser.getByRole("button", {
            name: "http_requests_total",
            exact: true,
          })
        ).toBeVisible()

        await toggle.click()
        await expect(browser).toHaveCount(0)
      })

      await test.step("settings", async () => {
        await editor(page)
          .getByRole("button", { name: "Add threshold" })
          .click()
        await editor(page)
          .getByRole("textbox", { name: "Warning from" })
          .fill("100")
        await expect(editor(page).getByText(/warning threshold/)).toBeVisible()
      })

      await test.step("JSON round trip", async () => {
        await viewSwitch(page).getByRole("radio", { name: "JSON" }).click()
        const json = editor(page).getByRole("textbox", { name: "Panel JSON" })
        await expect(json).toContainText('"ref": "B"')

        const current = JSON.parse(await json.innerText()) as Record<
          string,
          unknown
        >
        current.title = editedTitle
        await json.click()
        await page.keyboard.press(selectAll)
        await page.keyboard.insertText(JSON.stringify(current, null, 2))

        await viewSwitch(page).getByRole("radio", { name: "Editor" }).click()
        await expect(
          editor(page).getByRole("textbox", { name: "Title", exact: true })
        ).toHaveValue(editedTitle)
      })

      await test.step("keyboard", async () => {
        const editorRadio = viewSwitch(page).getByRole("radio", {
          name: "Editor",
        })
        await editorRadio.focus()
        await page.keyboard.press("ArrowRight")
        await expect(
          viewSwitch(page).getByRole("radio", { name: "JSON" })
        ).toBeChecked()
        await page.keyboard.press("ArrowLeft")
        await expect(editorRadio).toBeChecked()
      })

      await test.step("save with a note", async () => {
        await expect(editor(page).getByText(/^\d+ changes: /)).toBeVisible()
        await editor(page).getByRole("button", { name: "Add a note" }).click()
        await editor(page)
          .getByRole("textbox", { name: "Note for the timeline" })
          .fill(note)
        await editor(page)
          .getByRole("button", { name: "Save as version 2" })
          .click()

        await expect(
          card(page).getByRole("heading", { name: "Activity" })
        ).toBeVisible({
          timeout: 30_000,
        })
        const version = entryWith(page, "Version 2")
        await expect(version).toHaveCount(1)
        await expect(version).toContainText(note)
      })

      await test.step("apply to a newer version", async () => {
        await expect(panelMenu(page, editedTitle)).toBeVisible()
        await openEditor(page, editedTitle)
        await editor(page)
          .getByRole("textbox", { name: "Title", exact: true })
          .fill("Mine")

        const saved = await getDashboard(request, id)
        const logs = saved.spec.panels.map((item) =>
          item.id === "log_services"
            ? { ...item, title: "Logs, renamed" }
            : item
        )
        const newer = await addVersion(
          request,
          id,
          { ...saved.spec, panels: logs },
          saved.latestVersion
        )
        expect(newer).toBe(3)

        const apply = editor(page).getByRole("button", {
          name: "Apply my changes to version 3",
        })
        await expect(apply).toBeVisible({ timeout: 15_000 })
        await apply.click()

        await expect
          .poll(async () => (await getDashboard(request, id)).latestVersion)
          .toBe(4)
        const latest = (await getDashboard(request, id)).spec.panels
        expect(latest.find((item) => item.id === panelId)?.title).toBe("Mine")
        expect(latest.find((item) => item.id === "log_services")?.title).toBe(
          "Logs, renamed"
        )
      })

      await test.step("theme", async () => {
        await expect(page.locator("html")).toHaveClass(
          colorScheme === "dark" ? /\bdark\b/ : /^(?!.*\bdark\b)/
        )
      })
    })
  })
}
