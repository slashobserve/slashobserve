import { describe, expect, it } from "vitest"

import {
  askedHere,
  isAbout,
  isPrivateAsk,
  ownerOf,
  sitsOn,
  spotTarget,
} from "./canvas-host"

const dashboard = { kind: "dashboard", id: "d1" } as const
const observation = { kind: "observation", id: "o1" } as const
const spot = { kind: "position", pos: { x: 0.5, y: 0.5 } } as const

describe("sitsOn", () => {
  it("places a dashboard row by its panel and an observation row by its block", () => {
    expect(sitsOn(dashboard, "p1", { panel_id: "p1", block_id: null })).toBe(
      true
    )
    expect(sitsOn(dashboard, "p1", { panel_id: null, block_id: "p1" })).toBe(
      false
    )
    expect(sitsOn(observation, "b1", { panel_id: "p1", block_id: "b1" })).toBe(
      true
    )
    expect(sitsOn(observation, "b1", { panel_id: "b1", block_id: "b2" })).toBe(
      false
    )
  })
})

describe("ownerOf", () => {
  it("names a dashboard row's panel and an observation row's block", () => {
    const row = { panel_id: "p1", block_id: "b1" }
    expect(ownerOf(dashboard, row)).toBe("p1")
    expect(ownerOf(observation, row)).toBe("b1")
    expect(ownerOf(observation, { panel_id: "p1" })).toBeNull()
  })
})

describe("isAbout", () => {
  it("continues only a chat about the page's own dashboard or observation", () => {
    const onDashboard = { dashboard_id: "d1", observation_id: null }
    const onObservation = { dashboard_id: null, observation_id: "o1" }
    expect(isAbout(onDashboard, dashboard)).toBe(true)
    expect(isAbout(onDashboard, observation)).toBe(false)
    expect(isAbout(onObservation, observation)).toBe(true)
    expect(
      isAbout({ ...onObservation, observation_id: "o2" }, observation)
    ).toBe(false)
  })
})

describe("isPrivateAsk", () => {
  it("takes a person's question that writes into no observation", () => {
    expect(isPrivateAsk({ observation_id: null, trigger: "person" })).toBe(true)
    expect(isPrivateAsk({ observation_id: "o1", trigger: "person" })).toBe(
      false
    )
    expect(isPrivateAsk({ observation_id: null, trigger: "mention" })).toBe(
      false
    )
  })
})

describe("spotTarget", () => {
  it("names the panel of a dashboard and the block of an observation", () => {
    expect(spotTarget(dashboard, "p1", spot)).toEqual({
      panelId: "p1",
      anchor: spot,
    })
    expect(spotTarget(observation, "b1", spot)).toEqual({
      blockId: "b1",
      anchor: spot,
    })
  })

  it("leaves the anchor out for the whole panel or block", () => {
    expect(spotTarget(observation, "b1", null)).toEqual({ blockId: "b1" })
  })
})

describe("askedHere", () => {
  it("lists the anchored questions about the page", () => {
    expect(askedHere(dashboard)).toEqual({ dashboardId: "d1", anchored: true })
    expect(askedHere(observation)).toEqual({
      observationId: "o1",
      anchored: true,
    })
  })
})
