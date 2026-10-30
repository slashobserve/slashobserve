import { describe, expect, it } from "vitest"

import { createFieldResolver } from "@/panels/field-config"
import type { PreparedFrame } from "@/panels/transforms"
import { displayWithDefaults, type DataPanel } from "@/spec"

import {
  canvasModel,
  defaultLineColor,
  foregroundColor,
  type RgbOf,
} from "./canvas-model"

function panel(extra: Partial<DataPanel> = {}): DataPanel {
  return {
    id: "p1",
    title: "Canvas",
    gridPos: { x: 0, y: 0, w: 12, h: 8 },
    type: "canvas",
    datasource: "prom",
    queries: [{ query: "up" }],
    ...extra,
  } as DataPanel
}

function series(
  name: string,
  values: (number | null)[],
  field = "Value"
): PreparedFrame {
  return {
    ref: "A",
    name,
    labels: {},
    fields: [
      { name: "Time", type: "time", values: values.map((_, i) => i) },
      { name: field, type: "number", values },
    ],
  }
}

const rgbOf: RgbOf = (color) => {
  if (color === "#ffffff") return [255, 255, 255]
  if (color === "#000000") return [0, 0, 0]
  if (color === "var(--threshold-critical)") return [208, 59, 59]
  return null
}

const percentBody = { width: 100, height: 100 }

function build(
  frames: PreparedFrame[],
  display: Record<string, unknown>,
  extra: Partial<DataPanel> = {},
  body = percentBody
) {
  return canvasModel(
    frames,
    createFieldResolver(panel(extra)),
    displayWithDefaults("canvas", display),
    body,
    rgbOf
  )
}

const metric = (extra: Record<string, unknown> = {}) => ({
  id: "m",
  kind: "metric",
  left: 10,
  top: 10,
  width: 20,
  height: 20,
  ...extra,
})

const rect = (id: string, left: number, top: number, extra = {}) => ({
  id,
  kind: "rectangle",
  left,
  top,
  width: 20,
  height: 10,
  ...extra,
})

describe("canvasModel metrics", () => {
  it("reduces the first number field by display.reduce when no field is named", () => {
    const frames = [series("a", [1, 9, 4])]
    expect(build(frames, { elements: [metric()] }).elements[0].valueText).toBe(
      "4"
    )
    expect(
      build(frames, { reduce: "max", elements: [metric()] }).elements[0]
        .valueText
    ).toBe("9")
  })

  it("finds the named field by field name, series name or display name", () => {
    const frames = [series("a", [1], "cpu"), series("b", [7], "mem")]
    const named = (field: string) =>
      build(frames, { elements: [metric({ field })] }).elements[0].valueText
    expect(named("mem")).toBe("7")
    expect(named("a")).toBe("1")
  })

  it("shows No data for a missing field and when there is no data", () => {
    expect(
      build([series("a", [1])], { elements: [metric({ field: "nope" })] })
        .elements[0].valueText
    ).toBe("No data")
    expect(build([], { elements: [metric()] }).elements[0].valueText).toBe(
      "No data"
    )
  })

  it("keeps the label text apart from the value", () => {
    const [element] = build([series("a", [3])], {
      elements: [metric({ text: "Orders" })],
    }).elements
    expect(element.text).toBe("Orders")
    expect(element.valueText).toBe("3")
  })

  it("takes the threshold color and state of the field for field colors", () => {
    const [element] = build(
      [series("a", [95])],
      { elements: [metric({ textColor: "field" })] },
      { thresholds: [{ color: "ok" }, { value: 80, color: "critical" }] }
    ).elements
    expect(element.textColor).toBe("var(--threshold-critical)")
    expect(element.state).toEqual({
      level: "critical",
      color: "var(--threshold-critical)",
      label: "Critical",
      labelColor: foregroundColor,
    })
  })

  it("keeps the state word in the foreground color unless a fill sets the text color", () => {
    const thresholds = [{ color: "ok" }, { value: 80, color: "critical" }]
    const plain = build(
      [series("a", [95])],
      { elements: [metric({ textColor: "field" })] },
      { thresholds }
    ).elements[0]
    const filled = build(
      [series("a", [95])],
      { elements: [metric({ fill: "#ffffff", textColor: "field" })] },
      { thresholds }
    ).elements[0]
    const stroked = build(
      [series("a", [95])],
      { elements: [metric({ stroke: "field" })] },
      { thresholds }
    ).elements[0]
    expect(plain.state?.labelColor).toBe(foregroundColor)
    expect(filled.state?.labelColor).toBeNull()
    expect(stroked.state?.labelColor).toBeNull()
  })

  it("falls back to the series color when the field has no threshold or mapping color", () => {
    const frames = [series("a", [1]), series("b", [2])]
    const elements = build(frames, {
      elements: [
        metric({ id: "x", field: "a", textColor: "field" }),
        metric({ id: "y", field: "b", textColor: "field" }),
      ],
    }).elements
    expect(elements.map((element) => element.textColor)).toEqual([
      "var(--series-1)",
      "var(--series-2)",
    ])
    expect(elements[0].state).toBeNull()
  })

  it("uses a fixed color of the field as a state-less color", () => {
    const [element] = build(
      [series("a", [1])],
      { elements: [metric({ textColor: "field" })] },
      { color: "ok" }
    ).elements
    expect(element.textColor).toBe("var(--threshold-ok)")
    expect(element.state).toMatchObject({ level: "base", label: "" })
  })

  it("has no state unless a color asks for the field", () => {
    const [element] = build(
      [series("a", [95])],
      { elements: [metric()] },
      { thresholds: [{ color: "ok" }, { value: 80, color: "critical" }] }
    ).elements
    expect(element.state).toBeNull()
    expect(element.textColor).toBeNull()
  })
})

describe("canvasModel colors and text", () => {
  it("picks readable text on a fill and keeps an explicit text color", () => {
    const { elements } = build([], {
      elements: [
        rect("a", 0, 0, { fill: "#ffffff", text: "A" }),
        rect("b", 0, 20, { fill: "#000000", text: "B" }),
        rect("c", 0, 40, { fill: "#000000", textColor: "ok", text: "C" }),
        rect("d", 0, 60, { text: "D" }),
      ],
    })
    expect(elements.map((element) => element.textColor)).toEqual([
      "#0a0a0a",
      "#ffffff",
      "var(--threshold-ok)",
      null,
    ])
  })

  it("maps semantic colors to threshold tokens and keeps hex", () => {
    const [element] = build([], {
      elements: [rect("a", 0, 0, { fill: "warning", stroke: "#12abEF" })],
    }).elements
    expect(element.fill).toBe("var(--threshold-warning)")
    expect(element.stroke).toBe("#12abEF")
  })

  it("never lets a hostile color reach the output", () => {
    const hostile = [
      "red; background: url(//evil.example/x)",
      "url(javascript:alert(1))",
      "#12345",
      "var(--anything)",
      "rgb(0 0 0)",
      "expression(alert(1))",
      "FIELD",
      7,
    ]
    for (const color of hostile) {
      const model = build([series("a", [1])], {
        elements: [
          rect("a", 0, 0, { fill: color, stroke: color, textColor: color }),
          rect("b", 50, 50),
        ],
        connections: [{ from: "a", to: "b", color }],
      })
      expect(model.elements[0]).toMatchObject({
        fill: null,
        stroke: null,
        textColor: null,
      })
      expect(model.lines[0].color).toBe(defaultLineColor)
    }
  })

  it("passes element text through untouched for the renderer to print as text", () => {
    const html = '<svg onload="alert(1)"> <script>1</script>'
    const [element] = build([], {
      elements: [{ ...rect("a", 0, 0), kind: "text", text: html }],
    }).elements
    expect(element.text).toBe(html)
  })

  it("drops elements of an undeclared kind or with an invalid box", () => {
    const { elements } = build([], {
      elements: [
        { ...rect("a", 0, 0), kind: "image", text: "x" },
        { ...rect("b", 0, 0), kind: "gauge" },
        { ...rect("c", 0, 0), width: 0 },
        { ...rect("d", 0, 0), left: "5" },
        "text",
        null,
        rect("ok", 1, 1),
      ],
    })
    expect(elements.map((element) => element.id)).toEqual(["ok"])
  })

  it("applies the element defaults and ignores invalid size and alignment", () => {
    const [plain, odd] = build([], {
      elements: [
        rect("a", 0, 0),
        rect("b", 0, 0, { fontSize: 1000, align: "justify" }),
      ],
    }).elements
    expect(plain).toMatchObject({ fontSize: 14, align: "center" })
    expect(odd).toMatchObject({ fontSize: 72, align: "center" })
  })

  it("clamps the box into the panel", () => {
    const [element] = build([], {
      elements: [rect("a", 120, -5, { width: 400 })],
    }).elements
    expect(element.box).toEqual({ left: 100, top: 0, width: 100, height: 10 })
  })

  it("keeps Spec order", () => {
    const { elements } = build([], {
      elements: [rect("z", 0, 0), rect("a", 0, 20), rect("m", 0, 40)],
    })
    expect(elements.map((element) => element.id)).toEqual(["z", "a", "m"])
  })
})

describe("canvasModel connections", () => {
  it("runs from the facing edge of one box to the facing edge of the other", () => {
    const { lines } = build([], {
      elements: [rect("a", 0, 40), rect("b", 60, 40)],
      connections: [{ from: "a", to: "b" }],
    })
    expect(lines).toHaveLength(1)
    expect(lines[0]).toMatchObject({ x1: 20, y1: 45, x2: 60, y2: 45, width: 2 })
  })

  it("leaves an ellipse where the center line crosses its outline", () => {
    const { lines } = build([], {
      elements: [
        { ...rect("a", 0, 0), kind: "ellipse", width: 20, height: 20 },
        rect("b", 60, 5, { height: 10 }),
      ],
      connections: [{ from: "a", to: "b" }],
    })
    expect(lines[0].x1).toBeCloseTo(20)
    expect(lines[0].y1).toBeCloseTo(10)
  })

  it("leaves a rectangle through the top edge for a vertical line", () => {
    const { lines } = build([], {
      elements: [rect("a", 40, 0), rect("b", 40, 60)],
      connections: [{ from: "a", to: "b", width: 5, color: "critical" }],
    })
    expect(lines[0]).toMatchObject({
      x1: 50,
      y1: 10,
      x2: 50,
      y2: 60,
      width: 5,
      color: "var(--threshold-critical)",
    })
  })

  it("drops connections with a missing end or no length", () => {
    const { lines } = build([], {
      elements: [rect("a", 0, 0), rect("b", 0, 0)],
      connections: [
        { from: "a", to: "ghost" },
        { from: "ghost", to: "b" },
        { from: "a", to: "b" },
        { from: "a", to: "a" },
        { from: 1, to: 2 },
      ],
    })
    expect(lines).toEqual([])
  })

  it("colors a connection from its bound field", () => {
    const { lines } = build(
      [series("a", [95])],
      {
        elements: [rect("a", 0, 0), rect("b", 60, 60)],
        connections: [{ from: "a", to: "b", color: "field" }],
      },
      { thresholds: [{ color: "ok" }, { value: 80, color: "critical" }] }
    )
    expect(lines[0].color).toBe("var(--threshold-critical)")
  })
})

const textFrame = (
  field: string,
  values: (string | null)[]
): PreparedFrame => ({
  ref: "B",
  name: "labels",
  labels: {},
  fields: [
    { name: "Time", type: "time", values: values.map((_, i) => i) },
    { name: field, type: "string", values },
  ],
})

const newKinds = [
  "icon",
  "button",
  "server",
  "triangle",
  "cloud",
  "parallelogram",
  "windTurbine",
  "droneTop",
  "droneFront",
  "droneSide",
]

describe("canvasModel element kinds and styling", () => {
  it.each(newKinds)("accepts the %s kind", (kind) => {
    const { elements } = build([], {
      elements: [{ ...rect("a", 0, 0), kind }],
    })
    expect(elements.map((element) => element.kind)).toEqual([kind])
  })

  it("applies the element defaults of the Spec to a plain element", () => {
    const [element] = build([], { elements: [rect("a", 0, 0)] }).elements
    expect(element).toMatchObject({
      valign: "middle",
      rotation: 0,
      borderWidth: 1,
      radius: 0,
      ownRadius: null,
      background: null,
      icon: null,
      image: null,
      iconColor: null,
      accent: null,
      server: "single",
      buttonStyle: "primary",
      angle: 0,
      details: [],
    })
  })

  it("reads rotation, vertical alignment, border width and radius, clamped into the Spec ranges", () => {
    const [set, wild] = build([], {
      elements: [
        rect("a", 0, 0, {
          rotation: -45,
          valign: "bottom",
          borderWidth: 4,
          radius: 12,
        }),
        rect("b", 0, 0, {
          rotation: 900,
          valign: "diagonal",
          borderWidth: 99,
          radius: -3,
        }),
      ],
    }).elements
    expect(set).toMatchObject({
      rotation: -45,
      valign: "bottom",
      borderWidth: 4,
      radius: 12,
      ownRadius: 12,
    })
    expect(wild).toMatchObject({
      rotation: 360,
      valign: "middle",
      borderWidth: 20,
      radius: 0,
      ownRadius: 0,
    })
  })

  it("reads the icon name and colors it through the color validator", () => {
    const [icon, hostile] = build([], {
      elements: [
        rect("a", 0, 0, { kind: "icon", icon: "server", iconColor: "warning" }),
        rect("b", 0, 0, {
          kind: "icon",
          icon: "",
          iconColor: "url(javascript:1)",
        }),
      ],
    }).elements
    expect(icon).toMatchObject({
      icon: "server",
      iconColor: "var(--threshold-warning)",
    })
    expect(hostile).toMatchObject({ icon: null, iconColor: null })
  })

  it("takes an image from a URL, or from the last text of the bound field", () => {
    const frames = [
      textFrame("logo", [
        "https://a.example/1.png",
        "",
        "https://a.example/2.png",
        null,
      ]),
    ]
    const { elements } = build(frames, {
      elements: [
        rect("a", 0, 0, {
          kind: "icon",
          image: { url: "https://b.example/x.png" },
        }),
        rect("b", 0, 0, { kind: "icon", image: { field: "logo" } }),
        rect("c", 0, 0, { kind: "icon", image: { field: "missing" } }),
        rect("d", 0, 0, { kind: "icon", image: { field: 7 } }),
        rect("e", 0, 0, { kind: "icon", image: { url: "" } }),
      ],
    })
    expect(elements.map((element) => element.image)).toEqual([
      "https://b.example/x.png",
      "https://a.example/2.png",
      null,
      null,
      null,
    ])
  })

  it("does not read an image from a number field", () => {
    const { elements } = build([series("a", [1])], {
      elements: [rect("a", 0, 0, { kind: "icon", image: { field: "Value" } })],
    })
    expect(elements[0].image).toBeNull()
  })

  it("reads an element background and sizes it to contain by default", () => {
    const frames = [textFrame("map", ["https://a.example/map.png"])]
    const [sized, fallback, bound, none] = build(frames, {
      elements: [
        rect("a", 0, 0, {
          backgroundImage: { url: "https://a.example/b.png" },
          backgroundSize: "tile",
        }),
        rect("b", 0, 0, {
          backgroundImage: { url: "https://a.example/b.png" },
          backgroundSize: "stretch",
        }),
        rect("c", 0, 0, {
          backgroundImage: { field: "map" },
          backgroundSize: "cover",
        }),
        rect("d", 0, 0, { backgroundSize: "cover" }),
      ],
    }).elements
    expect(sized.background).toEqual({
      url: "https://a.example/b.png",
      size: "tile",
    })
    expect(fallback.background?.size).toBe("contain")
    expect(bound.background).toEqual({
      url: "https://a.example/map.png",
      size: "cover",
    })
    expect(none.background).toBeNull()
  })

  it("reads the server variant and a status light color", () => {
    const [stack, plain, hostile] = build([], {
      elements: [
        rect("a", 0, 0, {
          kind: "server",
          server: "stack",
          accentColor: "#112233",
        }),
        rect("b", 0, 0, { kind: "server", server: "rack" }),
        rect("c", 0, 0, { kind: "server", accentColor: "red" }),
      ],
    }).elements
    expect(stack).toMatchObject({ server: "stack", accent: "#112233" })
    expect(plain.server).toBe("single")
    expect(hostile.accent).toBeNull()
  })

  it("reads the button style and falls back to primary", () => {
    const styles = build([], {
      elements: ["success", "destructive", "secondary", "loud"].map(
        (style, i) =>
          rect(`b${i}`, 0, 0, {
            kind: "button",
            text: "Go",
            buttonStyle: style,
          })
      ),
    }).elements.map((element) => element.buttonStyle)
    expect(styles).toEqual(["success", "destructive", "secondary", "primary"])
  })
})

describe("canvasModel placement", () => {
  const body = { width: 400, height: 200 }

  it("turns the percent box into pixels of the body", () => {
    const [element] = build(
      [],
      { elements: [rect("a", 10, 20, { width: 25, height: 50 })] },
      {},
      body
    ).elements
    expect(element.box).toEqual({ left: 40, top: 40, width: 100, height: 100 })
  })

  it("lets a placement win over the percent box", () => {
    const [element] = build(
      [],
      {
        elements: [
          rect("a", 10, 20, {
            placement: {
              horizontal: "right",
              vertical: "bottom",
              right: 10,
              bottom: 5,
              width: 80,
              height: 30,
            },
          }),
        ],
      },
      {},
      body
    ).elements
    expect(element.box).toEqual({ left: 310, top: 165, width: 80, height: 30 })
  })

  it("ignores a placement the Spec would not accept", () => {
    const [element] = build(
      [],
      {
        elements: [
          rect("a", 10, 20, {
            width: 25,
            height: 50,
            placement: { horizontal: "middle", vertical: "top" },
          }),
        ],
      },
      {},
      body
    ).elements
    expect(element.box).toEqual({ left: 40, top: 40, width: 100, height: 100 })
  })

  it("starts a connection at the edge of a placed element", () => {
    const left = { horizontal: "left", vertical: "top" }
    const right = { horizontal: "right", vertical: "top" }
    const { lines } = build(
      [],
      {
        elements: [
          rect("a", 0, 0, {
            placement: { ...left, left: 0, top: 0, width: 100, height: 100 },
          }),
          rect("b", 0, 0, {
            placement: { ...right, right: 0, top: 0, width: 100, height: 100 },
          }),
        ],
        connections: [{ from: "a", to: "b" }],
      },
      {},
      body
    )
    expect(lines[0]).toMatchObject({ x1: 100, y1: 50, x2: 300, y2: 50 })
  })
})

describe("canvasModel tooltips and bound values", () => {
  it("lists the value of a metric and of any element naming a field", () => {
    const { elements } = build([series("a", [1, 7])], {
      elements: [
        metric(),
        rect("r", 0, 50, { field: "Value", fill: "field" }),
        rect("t", 0, 70, { kind: "text", text: "plain" }),
        rect("u", 0, 80, { fill: "field" }),
      ],
    })
    expect(elements[0].details).toHaveLength(1)
    expect(elements[0].details[0].text).toBe("7")
    expect(elements[1].details).toHaveLength(1)
    expect(elements[1].details[0].text).toBe("7")
    expect(elements[2].details).toEqual([])
    expect(elements[3].details).toEqual([])
  })

  it("adds the rate of a wind turbine", () => {
    const frames = [series("a", [1], "Value"), series("b", [42], "rpm")]
    const [turbine, other] = build(frames, {
      elements: [
        rect("a", 0, 0, { kind: "windTurbine", rateField: "rpm" }),
        rect("b", 0, 0, { kind: "rectangle", rateField: "rpm" }),
      ],
    }).elements
    expect(turbine.details.map((detail) => detail.text)).toEqual(["42"])
    expect(other.details).toEqual([])
  })

  it("rotates each drone by its own angle field in degrees", () => {
    const frames = [
      series("a", [10], "yaw"),
      series("b", [20], "roll"),
      series("c", [30], "pitch"),
    ]
    const drone = (id: string, kind: string) =>
      rect(id, 0, 0, {
        kind,
        yawField: "yaw",
        rollField: "roll",
        pitchField: "pitch",
      })
    const angles = build(frames, {
      elements: [
        drone("a", "droneTop"),
        drone("b", "droneFront"),
        drone("c", "droneSide"),
        drone("d", "rectangle"),
        rect("e", 0, 0, { kind: "droneTop", yawField: "missing" }),
        rect("f", 0, 0, { kind: "droneTop" }),
      ],
    }).elements.map((element) => element.angle)
    expect(angles).toEqual([10, 20, 30, 0, 0, 0])
  })
})

describe("canvasModel panel root", () => {
  it("has no look of its own by default", () => {
    expect(build([], {}).root).toEqual({
      backgroundColor: null,
      borderColor: null,
      borderWidth: 0,
      borderRadius: 0,
      background: null,
      panZoom: false,
      infinitePan: false,
      tooltip: false,
    })
  })

  it("reads colors, border, background and controls", () => {
    const { root } = build([], {
      backgroundColor: "info",
      borderColor: "#112233",
      borderWidth: 3,
      borderRadius: 8,
      backgroundImage: { url: "https://a.example/bg.png" },
      backgroundSize: "cover",
      panZoom: true,
      infinitePan: true,
      tooltip: "details",
    })
    expect(root).toEqual({
      backgroundColor: "var(--threshold-info)",
      borderColor: "#112233",
      borderWidth: 3,
      borderRadius: 8,
      background: { url: "https://a.example/bg.png", size: "cover" },
      panZoom: true,
      infinitePan: true,
      tooltip: true,
    })
  })

  it("keeps hostile colors and field colors out of the root", () => {
    const { root } = build([], {
      backgroundColor: "field",
      borderColor: "expression(alert(1))",
    })
    expect(root.backgroundColor).toBeNull()
    expect(root.borderColor).toBeNull()
  })
})

describe("canvasModel styled connections", () => {
  const two = [rect("a", 0, 40), rect("b", 60, 40)]
  const body = { width: 400, height: 200 }

  it("draws a straight solid line without arrows by default", () => {
    const [line] = build([], {
      elements: two,
      connections: [{ from: "a", to: "b" }],
    }).lines
    expect(line).toMatchObject({
      path: "straight",
      lineStyle: "solid",
      arrow: "none",
    })
    expect(line.points).toEqual([
      { x: 20, y: 45 },
      { x: 60, y: 45 },
    ])
  })

  it("reads path, line style and arrows, ignoring unknown values", () => {
    const lines = build([], {
      elements: two,
      connections: [
        {
          from: "a",
          to: "b",
          path: "curved",
          lineStyle: "dashed",
          arrow: "both",
        },
        { from: "a", to: "b", path: "zigzag", lineStyle: "wavy", arrow: "up" },
      ],
    }).lines
    expect(lines[0]).toMatchObject({
      path: "curved",
      lineStyle: "dashed",
      arrow: "both",
    })
    expect(lines[1]).toMatchObject({
      path: "straight",
      lineStyle: "solid",
      arrow: "none",
    })
  })

  it("bends through vertices given in percent of the body, clamped, and aims the ends at them", () => {
    const [line] = build(
      [],
      {
        elements: [rect("a", 0, 0), rect("b", 80, 0)],
        connections: [
          {
            from: "a",
            to: "b",
            vertices: [
              { x: 50, y: 100 },
              { x: "x", y: 5 },
              { x: 150, y: 5 },
              { x: 75, y: 90 },
            ],
          },
        ],
      },
      {},
      body
    ).lines
    expect(line.points.slice(1, -1)).toEqual([
      { x: 200, y: 200 },
      { x: 400, y: 10 },
      { x: 300, y: 180 },
    ])
    const start = line.points[0]
    const end = line.points[line.points.length - 1]
    expect(start.x).toBeGreaterThan(0)
    expect(start.y).toBeGreaterThan(0)
    expect(end.x).toBeLessThan(400)
    expect(end.y).toBeGreaterThan(0)
  })

  it("keeps a line between overlapping boxes when it has vertices", () => {
    const { lines } = build([], {
      elements: [rect("a", 0, 0), rect("b", 0, 0)],
      connections: [{ from: "a", to: "b", vertices: [{ x: 50, y: 50 }] }],
    })
    expect(lines).toHaveLength(1)
  })

  it("scales the width between the limits over the field range", () => {
    const frames = [series("a", [0, 100, 40], "rate")]
    const widthOf = (extra: Record<string, unknown>, panelExtra = {}) =>
      build(
        frames,
        {
          elements: two,
          connections: [{ from: "a", to: "b", widthField: "rate", ...extra }],
        },
        panelExtra
      ).lines[0].width
    expect(widthOf({})).toBeCloseTo(4.6)
    expect(widthOf({ minWidth: 2, maxWidth: 6 })).toBeCloseTo(3.6)
    expect(
      widthOf({ minWidth: 2, maxWidth: 6 }, { min: 0, max: 200 })
    ).toBeCloseTo(2.8)
  })

  it("uses the width key when the width field has no value", () => {
    const [missing, empty] = build([series("a", [null, null], "rate")], {
      elements: two,
      connections: [
        { from: "a", to: "b", width: 5, widthField: "ghost" },
        { from: "a", to: "b", width: 5, widthField: "rate" },
      ],
    }).lines
    expect(missing.width).toBe(5)
    expect(empty.width).toBe(5)
  })

  it("gives a constant field the narrow width", () => {
    const [line] = build([series("a", [7, 7], "rate")], {
      elements: two,
      connections: [
        { from: "a", to: "b", widthField: "rate", minWidth: 3, maxWidth: 9 },
      ],
    }).lines
    expect(line.width).toBe(3)
  })
})
