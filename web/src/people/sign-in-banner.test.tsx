import { screen } from "@testing-library/react"
import { afterEach, describe, expect, it, vi } from "vitest"

import { renderWithProviders } from "@/test/render"

import { SignInBanner } from "./sign-in-banner"

afterEach(() => vi.unstubAllGlobals())

function stubInstance(instance: Record<string, unknown>) {
  vi.stubGlobal(
    "fetch",
    vi.fn(
      async () =>
        new Response(JSON.stringify(instance), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        })
    )
  )
}

describe("SignInBanner", () => {
  it("says an unverified email can sign in when any domain is allowed", async () => {
    stubInstance({
      any_email_domain: true,
      allowed_email_domains: [],
      oidc_provider_name: "Dex",
    })
    renderWithProviders(<SignInBanner action={null} />)
    expect(
      await screen.findByText(
        "Anyone with an email address, verified or not, can sign in through Dex."
      )
    ).toBeVisible()
  })

  it("asks for a verified email when domains are listed", async () => {
    stubInstance({
      any_email_domain: false,
      allowed_email_domains: ["example.com"],
      oidc_provider_name: "Dex",
    })
    renderWithProviders(<SignInBanner action={null} />)
    expect(
      await screen.findByText(/Anyone with a verified email at/)
    ).toBeVisible()
  })
})
