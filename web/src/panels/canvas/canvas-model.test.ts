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

function build(
  frames: PreparedFrame[],
  display: Record<string, unknown>,
  extra: Partial<DataPanel> = {}
) {
  return canvasModel(
    frames,
    createFieldResolver(panel(extra)),
    displayWithDefaults("canvas", display),
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
        { ...rect("b", 0, 0), kind: "button" },
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
