import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { createFieldResolver } from "@/panels/field-config"
import type { ResolvedLink } from "@/panels/field-config"
import type { PreparedFrame } from "@/panels/transforms"
import { displayWithDefaults, type DataPanel } from "@/spec"

import type { ElementAction } from "./actions"
import { canvasModel } from "./canvas-model"
import {
  interactionOf,
  parseLinks,
  resolveElementLinks,
  type Interactive,
} from "./element-links"

const link = (title: string, href = `https://docs.example.com/${title}`) =>
  ({ title, href, newTab: false }) satisfies ResolvedLink

const action = (index: number, title = `Action ${index}`): ElementAction => ({
  index,
  title,
  method: "POST",
  host: "ops.example.com",
  confirm: null,
  variables: [],
  fields: {},
})

const element = (extra: Partial<Interactive> = {}): Interactive => ({
  kind: "text",
  oneClick: null,
  actions: [],
  ...extra,
})

describe("parseLinks", () => {
  it("keeps well-formed links and drops the rest", () => {
    expect(
      parseLinks([
        { title: "Docs", url: "https://docs.example.com", newTab: true },
        { title: "", url: "https://docs.example.com" },
        { title: "No url" },
        { title: "Plain", url: "/dashboards/x", newTab: "yes" },
        "text",
      ])
    ).toEqual([
      { title: "Docs", url: "https://docs.example.com", newTab: true },
      { title: "Plain", url: "/dashboards/x" },
    ])
  })

  it("returns nothing when links is not a list", () => {
    expect(parseLinks(undefined)).toEqual([])
    expect(parseLinks({ title: "x", url: "y" })).toEqual([])
  })
})

describe("resolveElementLinks", () => {
  beforeEach(() => {
    vi.stubGlobal("location", { origin: "https://observe.example.com" })
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  const scope = { vars: { region: ["eu"] }, from: "now-1h", to: "now" }

  it("interpolates variables and the element's bound value", () => {
    const [resolved] = resolveElementLinks(
      [
        {
          title: "Pods in ${region}",
          url: "https://logs.example.com/?r=${region}&v=${__value.raw}&t=${__value.text}",
          newTab: true,
        },
      ],
      { raw: "0.5", text: "50 %", numeric: 0.5 },
      scope
    )
    expect(resolved).toEqual({
      title: "Pods in eu",
      href: "https://logs.example.com/?r=eu&v=0.5&t=50%20%25",
      newTab: true,
    })
  })

  it("leaves out links whose protocol is not allowed", () => {
    const links = resolveElementLinks(
      [
        { title: "Bad", url: "javascript:alert(1)" },
        { title: "Data", url: "data:text/html,hi" },
        { title: "Good", url: "mailto:ops@example.com" },
      ],
      null,
      scope
    )
    expect(links.map((resolved) => resolved.title)).toEqual(["Good"])
  })
})

describe("interactionOf", () => {
  it("is not interactive without links or actions", () => {
    expect(interactionOf(element(), [])).toEqual({ mode: "none" })
  })

  it("makes a lone link or action the direct target", () => {
    expect(interactionOf(element(), [link("a")])).toEqual({
      mode: "direct",
      target: { kind: "link", link: link("a") },
    })
    expect(interactionOf(element({ actions: [action(0)] }), [])).toEqual({
      mode: "direct",
      target: { kind: "action", action: action(0) },
    })
  })

  it("lists links then actions in a menu when there is no oneClick", () => {
    const result = interactionOf(element({ actions: [action(0)] }), [
      link("a"),
      link("b"),
    ])
    expect(result).toEqual({
      mode: "menu",
      targets: [
        { kind: "link", link: link("a") },
        { kind: "link", link: link("b") },
        { kind: "action", action: action(0) },
      ],
    })
  })

  it("opens the first link directly with oneClick link", () => {
    const result = interactionOf(
      element({ oneClick: "link", actions: [action(0)] }),
      [link("a"), link("b")]
    )
    expect(result).toEqual({
      mode: "direct",
      target: { kind: "link", link: link("a") },
    })
  })

  it("runs the first action directly with oneClick action", () => {
    const result = interactionOf(
      element({ oneClick: "action", actions: [action(1), action(2)] }),
      [link("a")]
    )
    expect(result).toEqual({
      mode: "direct",
      target: { kind: "action", action: action(1) },
    })
  })

  it("falls back to the menu when oneClick names a kind the element lacks", () => {
    const result = interactionOf(element({ oneClick: "action" }), [
      link("a"),
      link("b"),
    ])
    expect(result.mode).toBe("menu")
  })

  it("keeps a dropped action's saved index on the one that stays", () => {
    const result = interactionOf(element({ actions: [action(3)] }), [])
    expect(result).toEqual({
      mode: "direct",
      target: { kind: "action", action: action(3) },
    })
  })

  describe("buttons", () => {
    it("run their first action and list several by title", () => {
      expect(
        interactionOf(element({ kind: "button", actions: [action(0)] }), [])
      ).toEqual({
        mode: "direct",
        target: { kind: "action", action: action(0) },
      })
      const several = interactionOf(
        element({ kind: "button", actions: [action(0), action(1)] }),
        [link("a")]
      )
      expect(several).toEqual({
        mode: "menu",
        targets: [
          { kind: "action", action: action(0) },
          { kind: "action", action: action(1) },
        ],
      })
    })

    it("ignore oneClick when they hold several actions", () => {
      const result = interactionOf(
        element({
          kind: "button",
          oneClick: "action",
          actions: [action(0), action(1)],
        }),
        []
      )
      expect(result.mode).toBe("menu")
    })

    it("have no action yet when nothing is saved", () => {
      expect(interactionOf(element({ kind: "button" }), [])).toEqual({
        mode: "none",
      })
    })
  })
})

describe("canvas model links and actions", () => {
  const panel = {
    id: "p1",
    title: "Canvas",
    gridPos: { x: 0, y: 0, w: 12, h: 8 },
    type: "canvas",
    datasource: "prom",
    queries: [{ query: "up" }],
  } as DataPanel

  const frames: PreparedFrame[] = [
    {
      ref: "A",
      name: "cpu",
      labels: {},
      fields: [
        { name: "Time", type: "time", values: [1, 2] },
        { name: "Value", type: "number", values: [0.25, 0.5] },
        { name: "pod", type: "string", values: ["checkout-0", "checkout-1"] },
      ],
    },
  ]

  function build(elements: unknown[]) {
    return canvasModel(
      frames,
      createFieldResolver(panel),
      displayWithDefaults("canvas", { elements }),
      { width: 100, height: 100 }
    )
  }

  const base = { id: "e", kind: "text", left: 0, top: 0, width: 50, height: 50 }

  it("binds the element's value and the referenced field to its actions", () => {
    const [shown] = build([
      {
        ...base,
        kind: "metric",
        oneClick: "action",
        links: [{ title: "Docs", url: "https://docs.example.com" }],
        actions: [
          { title: "Bad", url: "http://insecure.example.com/x" },
          {
            title: "Page",
            url: "https://ops.example.com/${__data.fields.pod}",
          },
        ],
      },
    ]).elements
    expect(shown.value).toEqual({ raw: "0.5", text: "0.5", numeric: 0.5 })
    expect(shown.oneClick).toBe("action")
    expect(shown.links).toEqual([
      { title: "Docs", url: "https://docs.example.com" },
    ])
    expect(shown.actions).toHaveLength(1)
    expect(shown.actions[0]).toMatchObject({
      index: 1,
      host: "ops.example.com",
      fields: { pod: "checkout-1" },
    })
  })

  it("has no value for an element bound to nothing", () => {
    const [plain] = build([base]).elements
    expect(plain.value).toBeNull()
    expect(plain.links).toEqual([])
    expect(plain.actions).toEqual([])
    expect(plain.oneClick).toBeNull()
  })

  it("ignores an unknown oneClick", () => {
    expect(
      build([{ ...base, oneClick: "menu" }]).elements[0].oneClick
    ).toBeNull()
  })
})
