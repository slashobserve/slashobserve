import { screen } from "@testing-library/react"
import { describe, expect, it } from "vitest"

import { renderWithProviders } from "@/test/render"

import { useObservationOrigin } from "./origin"

const observation = "8b1f6c1e-3f5a-4c2e-9a77-0c1d2e3f4a5b"

function Probe() {
  return <p data-testid="origin">{useObservationOrigin() ?? "none"}</p>
}

async function originAt(url: string) {
  renderWithProviders(<Probe />, { route: { path: "/", url } })
  return (await screen.findByTestId("origin")).textContent
}

describe("useObservationOrigin", () => {
  it("reads the observation the page was entered from", async () => {
    expect(await originAt(`/?observation=${observation}`)).toBe(observation)
  })

  it("is empty when the page was not entered from an observation", async () => {
    expect(await originAt("/")).toBe("none")
  })

  it("ignores a value that is not an observation id", async () => {
    expect(await originAt("/?observation=checkout-slow")).toBe("none")
  })
})
