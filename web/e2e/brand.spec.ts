import { expect, test } from "./fixtures"

const icons = [
  { path: "/favicon.svg", type: "image/svg+xml" },
  { path: "/favicon-32.png", type: "image/png" },
  { path: "/favicon-16.png", type: "image/png" },
  { path: "/apple-touch-icon.png", type: "image/png" },
]

for (const { path, type } of icons) {
  test(`serves ${path} as ${type}`, async ({ request }) => {
    const response = await request.get(path)
    expect(response.status()).toBe(200)
    expect(response.headers()["content-type"]).toMatch(
      new RegExp(`^${type.replace("+", "\\+")}`)
    )
  })
}

test("links the favicons and the app icon from the page head", async ({
  page,
}) => {
  await page.goto("/")
  for (const selector of [
    'link[rel="icon"][sizes="32x32"]',
    'link[rel="icon"][sizes="16x16"]',
    'link[rel="apple-touch-icon"]',
    'link[rel="icon"][type="image/svg+xml"]',
  ]) {
    await expect(page.locator(selector)).toHaveCount(1)
  }
})
