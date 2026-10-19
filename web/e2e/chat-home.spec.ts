import { mutationHeaders } from "./api"
import { expect, test } from "./fixtures"

const composerColors = {
  light: { background: "rgb(255, 255, 255)", border: "rgb(228, 228, 231)" },
  dark: { background: "rgb(17, 17, 19)", border: "rgb(42, 42, 46)" },
} as const

const unanswerablePrompt = "e2e: a follow-up the replay provider cannot answer"

for (const scheme of ["light", "dark"] as const) {
  test.describe(scheme, () => {
    test.use({ colorScheme: scheme })

    test("shows the chat home", async ({ page }) => {
      await page.goto("/")
      await expect(
        page.getByRole("heading", {
          level: 1,
          name: "What do you want to see?",
        })
      ).toBeVisible()
      const composer = page.getByRole("textbox", { name: "Message the agent" })
      await expect(composer).toBeFocused()

      const form = page.locator("main form")
      await expect(form.getByText(/^prom, healthy$/)).toBeVisible()
      await expect(form.getByText(/^ch, healthy$/)).toBeVisible()

      const examples = page
        .getByRole("group", { name: "Example prompts" })
        .getByRole("button")
      await expect.poll(() => examples.count()).toBeGreaterThanOrEqual(1)
      expect(await examples.count()).toBeLessThanOrEqual(4)

      await expect(form).toHaveCSS(
        "background-color",
        composerColors[scheme].background
      )
      await expect(form).toHaveCSS(
        "border-top-color",
        composerColors[scheme].border
      )
    })

    test("rejects an empty message", async ({ page }) => {
      await page.goto("/")
      await page.getByRole("button", { name: "Send" }).click()
      await expect(page.getByRole("alert")).toHaveText(
        "Describe the dashboard you want first."
      )
      await expect(page).toHaveURL(/\/$/)
    })

    test("Shift+Enter adds a line", async ({ page }) => {
      await page.goto("/")
      const composer = page.getByRole("textbox", { name: "Message the agent" })
      await composer.pressSequentially("a")
      await composer.press("Shift+Enter")
      await composer.pressSequentially("b")
      await expect(composer).toHaveValue("a\nb")
      await expect(page).toHaveURL(/\/$/)
    })

    test("lists recent chats", async ({ page }) => {
      const created = await page.request.post("/api/v1/observe", {
        data: { prompt: unanswerablePrompt },
        headers: mutationHeaders,
      })
      expect(created.status()).toBe(202)
      const { request } = (await created.json()) as { request: { id: string } }

      await page.goto("/")
      const recent = page.getByRole("region", { name: "Recent chats" })
      const row = recent
        .getByRole("button", { name: new RegExp(unanswerablePrompt) })
        .first()
      await expect(row).toBeVisible()
      await expect(row.locator('[data-status="critical"]')).toBeVisible({
        timeout: 30_000,
      })

      await row.click()
      await expect(page).toHaveURL(new RegExp(`/observe/${request.id}$`))
    })

    test("replaces the composer when no model is connected", async ({
      page,
    }) => {
      await page.route("**/api/v1/agent", (route) =>
        route.fulfill({
          json: { available: false, provider: null, model: null, effort: null },
        })
      )
      await page.goto("/")
      await expect(
        page.getByText("No model is connected to this server.")
      ).toBeVisible()
      await expect(
        page.getByRole("textbox", { name: "Message the agent" })
      ).toHaveCount(0)
      await expect(
        page.getByRole("group", { name: "Example prompts" })
      ).toHaveCount(0)
    })

    test.describe("on a phone", () => {
      test.use({ viewport: { width: 390, height: 844 } })

      test("fits a phone", async ({ page }) => {
        await page.goto("/")
        await expect(
          page.getByRole("heading", {
            level: 1,
            name: "What do you want to see?",
          })
        ).toBeVisible()
        await expect(page.getByRole("button", { name: "Send" })).toBeVisible()
        const fits = await page.evaluate(
          () => document.documentElement.scrollWidth <= window.innerWidth
        )
        expect(fits).toBe(true)
      })
    })
  })
}
