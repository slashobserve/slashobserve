import { authFile } from "./auth-file"
import { signInThroughDex } from "./dex"
import { test as setup } from "./fixtures"

setup("sign in through Dex", async ({ page }) => {
  await signInThroughDex(page)
  await page.context().storageState({ path: authFile })
})
