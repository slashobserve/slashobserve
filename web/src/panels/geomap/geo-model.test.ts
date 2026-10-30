import { afterEach, describe, expect, it, vi } from "vitest"

import { createFieldResolver } from "@/panels/field-config"
import type { PreparedFrame } from "@/panels/transforms"
import { displayWithDefaults, type DataPanel } from "@/spec"

import {
  fitChoice,
  fitPoints,
  geoModel,
  type GeoContext,
  initialView,
  layerNeeds,
  markerSymbols,
  maxMarkers,
  noLocationReason,
  skippedText,
  symbolPath,
  viewOptions,
  viewTuning,
} from "./geo-model"
import type { Gazetteers } from "./gazetteer/lookup"
import type { Feature } from "./layers/geojson"
import { clampZoom, fitBounds, project, worldView } from "./projection"
import { regionCorners } from "./regions"

function panel(extra: Partial<DataPanel> = {}): DataPanel {
  return {
    id: "g1",
    title: "Geo",
    gridPos: { x: 0, y: 0, w: 12, h: 8 },
    type: "geomap",
    datasource: "prom",
    queries: [{ query: "up" }],
    ...extra,
  } as DataPanel
}

type Column = {
  name: string
  type?: "number" | "string" | "time"
  values: unknown[]
}

function table(...columns: Column[]): PreparedFrame {
  return {
    ref: "A",
    labels: {},
    fields: columns.map(({ name, type, values }) => ({
      name,
      type: type ?? "number",
      values,
    })),
  } as PreparedFrame
}

function build(
  frames: PreparedFrame[],
  extra: Partial<DataPanel> = {},
  display: Record<string, unknown> = {}
) {
  return geoModel(
    frames,
    createFieldResolver(panel(extra)),
    displayWithDefaults("geomap", display)
  )
}

const cities = (extra: Column[] = []) =>
  table(
    { name: "lat", values: [48.85, 35.68, -33.87] },
    { name: "lon", values: [2.35, 139.69, 151.21] },
    ...extra
  )

describe("location fields", () => {
  it("finds lat and lon by name", () => {
    const model = build([cities()])
    expect(model.reason).toBeNull()
    expect(model.markers).toHaveLength(3)
    const paris = project(2.35, 48.85)
    const marker = model.markers.find((m) => m.label === "48.850, 2.350")
    expect(marker?.x).toBeCloseTo(paris.x, 10)
    expect(marker?.y).toBeCloseTo(paris.y, 10)
  })

  it("accepts latitude, lng and longitude spellings in any case", () => {
    const model = build([
      table({ name: "Latitude", values: [10] }, { name: "LNG", values: [20] }),
    ])
    expect(model.markers).toHaveLength(1)
  })

  it("reads numeric text such as Prometheus labels", () => {
    const model = build([
      table(
        { name: "lat", type: "string", values: ["10.5"] },
        { name: "lon", type: "string", values: ["20.25"] }
      ),
    ])
    expect(model.markers).toHaveLength(1)
  })

  it("decodes a geohash field", () => {
    const model = build([
      table({ name: "geohash", type: "string", values: ["u4pruydqqvj"] }),
    ])
    const point = project(10.40744, 57.64911)
    expect(model.markers[0].x).toBeCloseTo(point.x, 4)
    expect(model.markers[0].y).toBeCloseTo(point.y, 4)
  })

  it("prefers explicitly named fields over auto detection", () => {
    const frame = table(
      { name: "lat", values: [0] },
      { name: "lon", values: [0] },
      { name: "y", values: [50] },
      { name: "x", values: [10] }
    )
    const model = build([frame], {}, { latField: "y", lonField: "x" })
    expect(model.markers[0].label).toBe("50.000, 10.000")
  })

  it("uses the geohash field when only that is named", () => {
    const frame = table(
      { name: "lat", values: [0] },
      { name: "lon", values: [0] },
      { name: "cell", type: "string", values: ["ezs42"] }
    )
    const model = build([frame], {}, { geohashField: "cell" })
    expect(model.markers[0].label).toBe("42.605, -5.603")
  })

  it("follows the location option", () => {
    const frame = table(
      { name: "lat", values: [1] },
      { name: "lon", values: [2] },
      { name: "geohash", type: "string", values: ["ezs42"] }
    )
    expect(build([frame], {}, { location: "geohash" }).markers[0].label).toBe(
      "42.605, -5.603"
    )
    expect(build([frame], {}, { location: "coords" }).markers[0].label).toBe(
      "1.000, 2.000"
    )
  })

  it("explains what is missing when no location fields exist", () => {
    const model = build([table({ name: "value", values: [1, 2] })])
    expect(model.markers).toEqual([])
    expect(model.reason).toBe(noLocationReason)
  })

  it("explains it when a location mode finds nothing", () => {
    const model = build([cities()], {}, { location: "geohash" })
    expect(model.reason).toBe(noLocationReason)
  })
})

describe("skipped rows", () => {
  it("counts rows with missing, non-finite or out-of-range coordinates", () => {
    const model = build([
      table(
        { name: "lat", values: [1, null, NaN, 91, 5, "x"] },
        { name: "lon", values: [1, 2, 3, 4, 181, 6] }
      ),
    ])
    expect(model.markers).toHaveLength(1)
    expect(model.skipped).toBe(5)
  })

  it("counts an invalid geohash", () => {
    const model = build([
      table({ name: "geohash", type: "string", values: ["ezs42", "!!", 7] }),
    ])
    expect(model.markers).toHaveLength(1)
    expect(model.skipped).toBe(2)
  })

  it("words the count in singular and plural", () => {
    expect(skippedText(1)).toBe("1 row had no location.")
    expect(skippedText(12)).toBe("12 rows had no location.")
  })

  it("keeps at most 10,000 markers and counts the rest", () => {
    const total = maxMarkers + 5
    const model = build([
      table(
        { name: "lat", values: Array.from({ length: total }, () => 1) },
        { name: "lon", values: Array.from({ length: total }, () => 2) }
      ),
    ])
    expect(model.markers).toHaveLength(maxMarkers)
    expect(model.truncated).toBe(5)
  })
})

describe("marker size", () => {
  it("scales linearly from the small to the large size over the range", () => {
    const model = build(
      [cities([{ name: "value", values: [10, 20, 30] }])],
      {},
      { markerSize: 4, markerSizeMax: 12 }
    )
    const radii = model.markers.map((m) => m.radius).sort((a, b) => a - b)
    expect(radii).toEqual([4, 8, 12])
  })

  it("draws the largest markers first", () => {
    const model = build([cities([{ name: "value", values: [10, 30, 20] }])])
    const radii = model.markers.map((m) => m.radius)
    expect(radii).toEqual([...radii].sort((a, b) => b - a))
  })

  it("uses the named size field", () => {
    const model = build(
      [
        cities([
          { name: "first", values: [1, 1, 1] },
          { name: "count", values: [0, 5, 10] },
        ]),
      ],
      {},
      { sizeField: "count", markerSize: 2, markerSizeMax: 10 }
    )
    expect(model.markers.map((m) => m.radius).sort((a, b) => a - b)).toEqual([
      2, 6, 10,
    ])
  })

  it("uses the small size when every value is equal or there is no number field", () => {
    const equal = build([cities([{ name: "value", values: [3, 3, 3] }])])
    expect(equal.markers.every((m) => m.radius === 5)).toBe(true)
    const none = build([cities()])
    expect(none.markers.every((m) => m.radius === 5)).toBe(true)
  })

  it("never lets the large size fall below the small size", () => {
    const model = build(
      [cities([{ name: "value", values: [1, 2, 3] }])],
      {},
      { markerSize: 9, markerSizeMax: 3 }
    )
    expect(model.markers.every((m) => m.radius === 9)).toBe(true)
  })
})

describe("marker color", () => {
  const thresholds = [
    { color: "ok" as const },
    { value: 15, color: "warning" as const },
    { value: 25, color: "critical" as const },
  ]

  it("takes the threshold color of the color field and keeps its state", () => {
    const model = build([cities([{ name: "value", values: [10, 20, 30] }])], {
      thresholds,
    })
    const byRadius = [...model.markers].sort((a, b) => a.radius - b.radius)
    expect(byRadius.map((m) => m.color)).toEqual([
      "var(--threshold-ok)",
      "var(--threshold-warning)",
      "var(--threshold-critical)",
    ])
    expect(byRadius[2].state).toEqual({ level: "critical", word: "Critical" })
  })

  it("takes a value mapping color without a threshold state", () => {
    const model = build([cities([{ name: "value", values: [1, 1, 1] }])], {
      mappings: [{ value: "1", text: "Up", color: "ok" }],
    } as Partial<DataPanel>)
    expect(model.markers[0].color).toBe("var(--threshold-ok)")
    expect(model.markers[0].state).toBeNull()
  })

  it("falls back to the first series color", () => {
    const model = build([cities([{ name: "value", values: [1, 2, 3] }])])
    expect(new Set(model.markers.map((m) => m.color))).toEqual(
      new Set(["var(--series-1)"])
    )
    expect(model.markers[0].state).toBeNull()
  })
})

describe("tooltip details", () => {
  it("lists the other fields with formatted values", () => {
    const model = build(
      [
        cities([
          {
            name: "city",
            type: "string",
            values: ["Paris", "Tokyo", "Sydney"],
          },
          { name: "value", values: [1, 2, 3] },
        ]),
      ],
      { unit: "percent" } as Partial<DataPanel>
    )
    const paris = model.markers.find((m) => m.label === "48.850, 2.350")
    expect(paris?.details.map((d) => d.name)).toEqual(["city", "value"])
    expect(paris?.details[0].text).toBe("Paris")
    expect(paris?.details[1].text).toContain("1")
  })
})

describe("viewOptions", () => {
  it("defaults to fitting the data", () => {
    expect(viewOptions({}).mode).toBe("fit")
  })

  it("reads the coordinate view", () => {
    expect(viewOptions({ view: "coords", lat: 10, lon: 20, zoom: 5 })).toEqual({
      mode: "coords",
      lat: 10,
      lon: 20,
      zoom: 5,
    })
  })
})

describe("initialView", () => {
  const { markers } = build([cities()])
  const size = [600, 300] as const

  it("fits the markers by default", () => {
    expect(initialView(viewOptions({}), markers, ...size)).toEqual(
      fitBounds(markers, ...size)
    )
  })

  it("shows the world on request", () => {
    expect(
      initialView(viewOptions({ view: "world" }), markers, ...size)
    ).toEqual(worldView(...size))
  })

  it("centers on the given coordinates at the given zoom", () => {
    const view = initialView(
      viewOptions({ view: "coords", lat: 48, lon: 2, zoom: 7 }),
      markers,
      ...size
    )
    expect(view).toEqual({ center: project(2, 48), zoom: 7 })
  })

  it("keeps a coordinate view inside the zoom limits", () => {
    const view = initialView(
      viewOptions({ view: "coords", zoom: 99 }),
      markers,
      ...size
    )
    expect(view.zoom).toBe(18)
  })
})

const onQuery = (ref: string, frame: PreparedFrame): PreparedFrame => ({
  ...frame,
  ref,
})

describe("layers", () => {
  const paris = onQuery(
    "A",
    table(
      { name: "lat", values: [48.85] },
      { name: "lon", values: [2.35] },
      { name: "city", type: "string", values: ["Paris"] }
    )
  )
  const tokyo = onQuery(
    "B",
    table({ name: "lat", values: [35.68] }, { name: "lon", values: [139.69] })
  )

  it("draws one markers layer from the flat keys when layers are absent", () => {
    const model = build([cities()])
    expect(model.layers).toHaveLength(1)
    expect(model.layers[0]).toMatchObject({ kind: "markers", name: "markers" })
    expect(model.markers).toHaveLength(3)
    expect(model.notes).toEqual([])
  })

  it("draws two markers layers, each from its own query", () => {
    const model = build(
      [paris, tokyo],
      {},
      {
        layers: [
          { type: "markers", name: "Cities", query: "A", color: "critical" },
          { type: "markers", name: "Sites", query: "B", color: "#112233" },
        ],
      }
    )
    const [first, second] = model.layers
    expect(first).toMatchObject({ kind: "markers", name: "Cities" })
    expect(second).toMatchObject({ kind: "markers", name: "Sites" })
    if (first.kind !== "markers" || second.kind !== "markers") return
    expect(first.markers.map((m) => m.label)).toEqual(["48.850, 2.350"])
    expect(second.markers.map((m) => m.label)).toEqual(["35.680, 139.690"])
    expect(first.markers[0].color).toBe("var(--threshold-critical)")
    expect(second.markers[0].color).toBe("#112233")
    expect(model.markers).toHaveLength(2)
  })

  it("keeps a layer with no query on every frame", () => {
    const model = build([paris, tokyo], {}, { layers: [{ type: "markers" }] })
    expect(model.markers).toHaveLength(2)
  })

  it("lets a color field win over the fixed color", () => {
    const frame = table(
      { name: "lat", values: [1] },
      { name: "lon", values: [2] },
      { name: "load", values: [95] }
    )
    const model = build(
      [frame],
      { thresholds: [{ color: "ok" }, { value: 90, color: "critical" }] },
      { layers: [{ type: "markers", color: "#112233", colorField: "load" }] }
    )
    expect(model.markers[0].color).toBe("var(--threshold-critical)")
    expect(model.markers[0].state?.level).toBe("critical")
  })

  it("reads the symbol, rotation and label text", () => {
    const frame = table(
      { name: "lat", values: [10, 20] },
      { name: "lon", values: [30, 40] },
      { name: "heading", values: [90, 180] },
      { name: "name", type: "string", values: ["a", "b"] }
    )
    const model = build(
      [frame],
      {},
      {
        layers: [
          {
            type: "markers",
            symbol: "triangle",
            textField: "name",
            textSize: 14,
            rotationField: "heading",
            opacity: 0.5,
            tooltip: false,
          },
        ],
      }
    )
    expect(model.layers[0]).toMatchObject({
      kind: "markers",
      textSize: 14,
      opacity: 0.5,
      tooltip: false,
    })
    const drawn = model.markers.map((m) => [m.symbol, m.rotation, m.text])
    expect(drawn).toHaveLength(2)
    expect(drawn).toContainEqual(["triangle", 90, "a"])
    expect(drawn).toContainEqual(["triangle", 180, "b"])
  })

  it("falls back to a circle for an unknown symbol", () => {
    const model = build(
      [cities()],
      {},
      { layers: [{ type: "markers", symbol: "hexagon" }] }
    )
    expect(model.markers[0].symbol).toBe("circle")
  })

  it("draws tile layers in order with the markers", () => {
    const model = build(
      [cities()],
      {},
      {
        layers: [
          { type: "markers" },
          { type: "osm", opacity: 0.4 },
          { type: "carto", theme: "dark", labels: false },
        ],
      }
    )
    expect(model.layers.map((layer) => layer.kind)).toEqual([
      "markers",
      "tiles",
      "tiles",
    ])
    const carto = model.layers[2]
    expect(carto.kind === "tiles" && carto.source.template).toContain(
      "dark_nolabels"
    )
    expect(model.layers[1].opacity).toBe(0.4)
  })

  it("follows the app theme for an automatic CARTO layer", () => {
    const model = geoModel(
      [cities()],
      createFieldResolver(panel()),
      displayWithDefaults("geomap", { layers: [{ type: "carto" }] }),
      "dark"
    )
    const layer = model.layers[0]
    expect(layer.kind === "tiles" && layer.source.template).toContain(
      "dark_all"
    )
  })

  it("shows the map, not the no-location reason, when only tiles are drawn", () => {
    const model = build([], {}, { layers: [{ type: "osm" }] })
    expect(model.reason).toBeNull()
    expect(model.layers).toHaveLength(1)
  })

  it("explains an unusable custom tile layer and still draws the rest", () => {
    const model = build(
      [cities()],
      {},
      {
        layers: [
          { type: "xyz", name: "Weather", url: "https://x.test/tile.png" },
          { type: "markers" },
        ],
      }
    )
    expect(model.notes).toEqual([
      "Layer Weather needs an https URL with {z}, {x} and {y}.",
    ])
    expect(model.markers).toHaveLength(3)
  })

  it("explains missing locations when every markers layer finds none", () => {
    const model = build(
      [table({ name: "value", values: [1] })],
      {},
      { layers: [{ type: "markers" }, { type: "markers" }] }
    )
    expect(model.reason).toBe(noLocationReason)
  })

  it("shares the marker cap across layers", () => {
    const rows = maxMarkers - 1
    const big = table(
      { name: "lat", values: Array.from({ length: rows }, () => 1) },
      { name: "lon", values: Array.from({ length: rows }, () => 2) }
    )
    const model = build(
      [big],
      {},
      { layers: [{ type: "markers" }, { type: "markers" }] }
    )
    expect(model.markers).toHaveLength(maxMarkers)
    expect(model.truncated).toBe(rows - 1)
  })
})

describe("layer types", () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  function withContext(
    frames: PreparedFrame[],
    layers: Record<string, unknown>[],
    context: GeoContext = {}
  ) {
    return geoModel(
      frames,
      createFieldResolver(panel()),
      displayWithDefaults("geomap", { layers }),
      "light",
      context
    )
  }

  const kinds = (model: ReturnType<typeof withContext>) =>
    model.layers.map((layer) => layer.kind)

  it("notes a type the map does not know and draws the rest", () => {
    const model = withContext(
      [cities()],
      [{ type: "hologram", name: "Holo" }, { type: "markers" }]
    )
    expect(model.notes).toEqual(["Layer Holo is not a layer this map knows."])
    expect(model.markers).toHaveLength(3)
  })

  it("draws a heatmap from the same locations, weighted by a field", () => {
    const model = withContext(
      [cities([{ name: "load", values: [1, 2, 4] }])],
      [{ type: "heatmap", weightField: "load" }]
    )
    const layer = model.layers[0]
    expect(layer.kind).toBe("heatmap")
    expect(
      layer.kind === "heatmap" && layer.points.map((p) => p.weight)
    ).toEqual([0.25, 0.5, 1])
    expect(model.markers).toEqual([])
    expect(model.reason).toBeNull()
  })

  it("weights every heatmap point equally without a weight field", () => {
    const layer = withContext([cities()], [{ type: "heatmap" }]).layers[0]
    expect(
      layer.kind === "heatmap" && layer.points.map((p) => p.weight)
    ).toEqual([1, 1, 1])
  })

  it("draws a route through each series, ordered by its time field", () => {
    const track = table(
      { name: "lat", values: [1, 2, 3] },
      { name: "lon", values: [10, 20, 30] },
      { name: "time", type: "time", values: [300, 100, 200] }
    )
    const layer = withContext([track], [{ type: "route" }]).layers[0]
    expect(layer.kind).toBe("route")
    expect(
      layer.kind === "route" && layer.lines[0].points.map((p) => p.lat)
    ).toEqual([2, 3, 1])
  })

  it("draws photos from an image field through the image proxy", () => {
    vi.stubGlobal("location", { origin: "https://app.test" })
    const shots = cities([
      {
        name: "image",
        type: "string",
        values: [
          "https://img.example/a.png",
          "http://img.example/b.png",
          "https://img.example/c.png",
        ],
      },
    ])
    const model = withContext(
      [shots],
      [{ type: "photos", imageField: "image" }]
    )
    const layer = model.layers[0]
    expect(layer.kind).toBe("photos")
    expect(layer.kind === "photos" && layer.photos.map((p) => p.row)).toEqual([
      0, 2,
    ])
    expect(
      layer.kind === "photos" &&
        layer.photos.every((p) => p.href.startsWith("/api/v1/assets/image"))
    ).toBe(true)
    expect(model.notes).toEqual([])
  })

  it("asks for an image field when a photos layer has none", () => {
    const model = withContext([cities()], [{ type: "photos", name: "Shots" }])
    expect(model.notes).toEqual(["Layer Shots needs an image field."])
  })

  describe("network", () => {
    const nodes = onQuery(
      "N",
      table(
        { name: "id", type: "string", values: ["a", "b", "c"] },
        { name: "lat", values: [10, 20, 30] },
        { name: "lon", values: [1, 2, 3] }
      )
    )
    const edges = onQuery(
      "E",
      table(
        { name: "source", type: "string", values: ["a", "b", "a"] },
        { name: "target", type: "string", values: ["b", "c", "ghost"] }
      )
    )

    it("draws edges between located nodes and the nodes as markers", () => {
      const model = withContext([nodes, edges], [{ type: "network" }])
      expect(kinds(model)).toEqual(["network", "markers"])
      const layer = model.layers[0]
      expect(layer.kind === "network" && layer.edges).toHaveLength(2)
      expect(model.markers).toHaveLength(3)
    })

    it("counts an edge that reaches an unknown node as skipped", () => {
      const model = withContext([nodes, edges], [{ type: "network" }])
      expect(model.skipped).toBe(1)
    })

    it("reads nodes and edges from its own query only", () => {
      const model = withContext(
        [nodes, edges],
        [{ type: "network", query: "N" }]
      )
      const layer = model.layers[0]
      expect(layer.kind === "network" && layer.edges).toHaveLength(0)
      expect(model.markers).toHaveLength(3)
    })
  })

  describe("day and night", () => {
    it("draws without any data, at the end of the time range", () => {
      const model = withContext([], [{ type: "dayNight", showSun: true }], {
        time: Date.parse("2024-03-20T12:00:00Z"),
      })
      expect(model.reason).toBeNull()
      const layer = model.layers[0]
      expect(layer.kind).toBe("dayNight")
      expect(layer.kind === "dayNight" && layer.sun).not.toBeNull()
    })

    it("keeps a chosen night color", () => {
      const layer = withContext(
        [],
        [{ type: "dayNight", nightColor: "info" }],
        { time: 0 }
      ).layers[0]
      expect(layer.kind === "dayNight" && layer.color).toBe(
        "var(--threshold-info)"
      )
    })
  })

  describe("geojson", () => {
    const outline: Feature = {
      properties: { name: "Box" },
      geometry: {
        type: "Polygon",
        coordinates: [
          [
            [0, 0],
            [10, 0],
            [10, 10],
            [0, 10],
            [0, 0],
          ],
        ],
      },
    }
    const url = "https://data.example/a.geojson"

    it("draws a bundled source once it has loaded", () => {
      const loaded = withContext(
        [],
        [{ type: "geojson", source: "countries" }],
        { atlases: { countries: [outline] } }
      )
      const layer = loaded.layers[0]
      expect(layer.kind === "geojson" && layer.features).toHaveLength(1)
      expect(loaded.reason).toBeNull()
      const loading = withContext(
        [],
        [{ type: "geojson", source: "countries" }]
      )
      const pending = loading.layers[0]
      expect(pending.kind === "geojson" && pending.features).toHaveLength(0)
      expect(loading.notes).toEqual([])
    })

    it("draws a remote file when it is ready", () => {
      const model = withContext([], [{ type: "geojson", url }], {
        remote: { [url]: { state: "ready", features: [outline] } },
      })
      const layer = model.layers[0]
      expect(layer.kind === "geojson" && layer.features).toHaveLength(1)
    })

    it("lists a blocked host and draws nothing while it loads or fails", () => {
      const blocked = withContext([], [{ type: "geojson", url }], {
        remote: { [url]: { state: "blocked", host: "data.example" } },
      })
      expect(blocked.blocked).toEqual(["data.example"])
      const failed = withContext(
        [],
        [{ type: "geojson", url, name: "Zones" }],
        {
          remote: { [url]: { state: "failed" } },
        }
      )
      expect(failed.notes).toEqual(["Layer Zones could not load its GeoJSON."])
      const loading = withContext([], [{ type: "geojson", url }], {
        remote: { [url]: { state: "loading" } },
      })
      expect(loading.notes).toEqual([])
      expect(loading.blocked).toEqual([])
    })

    it("asks for an https URL, and for a source when there is none", () => {
      expect(
        withContext(
          [],
          [{ type: "geojson", name: "Zones", url: "http://x.test/a.json" }]
        ).notes
      ).toEqual(["Layer Zones needs an https URL."])
      expect(
        withContext([], [{ type: "geojson", name: "Zones" }]).notes
      ).toEqual(["Layer Zones needs a URL or a bundled source."])
    })

    it("says so when the file is too large to draw", () => {
      const many = Array.from({ length: 5_001 }, () => outline)
      const model = withContext([], [{ type: "geojson", url }], {
        remote: { [url]: { state: "ready", features: many } },
      })
      expect(model.notes).toEqual(["This GeoJSON is too large to draw."])
    })

    it("colors features by the first matching rule", () => {
      const model = withContext(
        [],
        [
          {
            type: "geojson",
            source: "countries",
            rules: [
              { property: "name", operation: "eq", value: "Box", color: "ok" },
            ],
          },
        ],
        { atlases: { countries: [outline] } }
      )
      const layer = model.layers[0]
      expect(layer.kind === "geojson" && layer.features[0].style.fill).toBe(
        "var(--threshold-ok)"
      )
    })
  })

  describe("lookup locations", () => {
    const places = table(
      { name: "country", type: "string", values: ["France", "Atlantis"] },
      { name: "value", values: [1, 2] }
    )
    const gazetteers: Gazetteers = {
      countries: {
        find: (value) => (value === "France" ? { lon: 2, lat: 46 } : null),
      },
    }

    it("places rows through the gazetteer and skips the ones it cannot find", () => {
      const model = withContext(
        [places],
        [{ type: "markers", location: "lookup", lookupField: "country" }],
        { gazetteers }
      )
      expect(model.markers).toHaveLength(1)
      expect(model.markers[0].lon).toBeCloseTo(2, 10)
      expect(model.skipped).toBe(1)
    })

    it("uses the first text field when no lookup field is named", () => {
      const model = withContext(
        [places],
        [{ type: "markers", location: "lookup" }],
        { gazetteers }
      )
      expect(model.markers).toHaveLength(1)
    })

    it("stays quiet while the gazetteer loads", () => {
      const model = withContext(
        [places],
        [{ type: "markers", location: "lookup" }],
        {}
      )
      expect(model.markers).toEqual([])
      expect(model.skipped).toBe(0)
      expect(model.notes).toEqual([])
      expect(model.reason).toBeNull()
    })

    it("says airports are not available and skips the rows", () => {
      const model = withContext(
        [places],
        [{ type: "markers", location: "lookup", gazetteer: "airports" }],
        { gazetteers }
      )
      expect(model.markers).toEqual([])
      expect(model.skipped).toBe(2)
      expect(model.notes).toEqual(["Airport lookup is not available."])
    })

    it("feeds heatmaps the same way", () => {
      const model = withContext(
        [places],
        [{ type: "heatmap", location: "lookup" }],
        { gazetteers }
      )
      const layer = model.layers[0]
      expect(layer.kind === "heatmap" && layer.points).toHaveLength(1)
    })
  })

  describe("without locations", () => {
    it("explains when a heatmap finds none", () => {
      const model = withContext(
        [table({ name: "value", values: [1] })],
        [{ type: "heatmap" }]
      )
      expect(model.reason).toBe(noLocationReason)
    })

    it("does not explain when a standalone layer still draws", () => {
      const model = withContext(
        [table({ name: "value", values: [1] })],
        [{ type: "heatmap" }, { type: "dayNight" }]
      )
      expect(model.reason).toBeNull()
    })
  })

  it("shares the location budget between marker and heatmap layers", () => {
    const rows = maxMarkers - 1
    const big = table(
      { name: "lat", values: Array.from({ length: rows }, () => 1) },
      { name: "lon", values: Array.from({ length: rows }, () => 2) }
    )
    const model = withContext([big], [{ type: "heatmap" }, { type: "markers" }])
    const heat = model.layers[0]
    expect(heat.kind === "heatmap" && heat.points).toHaveLength(rows)
    expect(model.markers).toHaveLength(maxMarkers - rows)
    expect(model.truncated).toBe(rows - (maxMarkers - rows))
  })
})

describe("layerNeeds", () => {
  const url = "https://data.example/a.geojson"

  it("lists the bundled sources and remote files the layers read", () => {
    expect(
      layerNeeds({
        layers: [
          { type: "geojson", source: "usa-states" },
          { type: "geojson", url },
          { type: "markers", location: "lookup", gazetteer: "countries" },
          { type: "markers", location: "lookup" },
        ],
      })
    ).toEqual({ atlases: ["usa-states", "countries"], urls: [url] })
  })

  it("prefers a URL over a bundled source and ignores unusable URLs", () => {
    expect(
      layerNeeds({
        layers: [
          { type: "geojson", source: "countries", url },
          { type: "geojson", url: "http://x.test/a.json" },
        ],
      })
    ).toEqual({ atlases: [], urls: [url] })
  })

  it("needs nothing for airports, coordinates or other layers", () => {
    expect(
      layerNeeds({
        layers: [
          { type: "markers", location: "lookup", gazetteer: "airports" },
          { type: "markers" },
          { type: "heatmap" },
          { type: "dayNight" },
        ],
      })
    ).toEqual({ atlases: [], urls: [] })
  })

  it("reads the flat display of a map that has no layers list", () => {
    expect(layerNeeds({})).toEqual({ atlases: [], urls: [] })
  })
})

describe("fitPoints", () => {
  const first = onQuery(
    "A",
    table({ name: "lat", values: [10, 20] }, { name: "lon", values: [1, 2] })
  )
  const second = onQuery(
    "B",
    table({ name: "lat", values: [30, 40] }, { name: "lon", values: [3, 4] })
  )
  const model = build(
    [first, second],
    {},
    {
      layers: [
        { type: "markers", name: "one", query: "A" },
        { type: "markers", name: "two", query: "B" },
      ],
    }
  )

  it("fits every markers layer by default", () => {
    expect(fitPoints(model, { lastOnly: false })).toHaveLength(4)
  })

  it("fits only the named layer", () => {
    const points = fitPoints(model, { layer: "two", lastOnly: false })
    expect(points.map((p) => p.lat).sort()).toEqual([30, 40])
  })

  it("falls back to every layer when the name matches none", () => {
    expect(
      fitPoints(model, { layer: "missing", lastOnly: false })
    ).toHaveLength(4)
  })

  it("keeps the last row of each series when asked", () => {
    const points = fitPoints(model, { lastOnly: true })
    expect(points.map((p) => p.lat).sort()).toEqual([20, 40])
  })

  it("fits the layers that place things beyond markers", () => {
    const url = "https://data.example/a.geojson"
    const wide = geoModel(
      [
        table(
          { name: "lat", values: [5, 6] },
          { name: "lon", values: [7, 8] },
          { name: "time", type: "time", values: [1, 2] }
        ),
      ],
      createFieldResolver(panel()),
      displayWithDefaults("geomap", {
        layers: [
          { type: "heatmap", name: "heat" },
          { type: "route", name: "path" },
          { type: "dayNight", name: "night" },
          { type: "geojson", name: "shape", url },
        ],
      }),
      "light",
      {
        remote: {
          [url]: {
            state: "ready",
            features: [
              {
                properties: {},
                geometry: {
                  type: "LineString",
                  coordinates: [
                    [-20, -10],
                    [30, 40],
                  ],
                },
              },
            ],
          },
        },
      }
    )
    expect(fitPoints(wide, { lastOnly: false })).toHaveLength(2 + 2 + 2)
    expect(fitPoints(wide, { layer: "shape", lastOnly: false })).toEqual([
      { lon: -20, lat: -10 },
      { lon: 30, lat: 40 },
    ])
    expect(fitPoints(wide, { layer: "night", lastOnly: false })).toHaveLength(6)
    expect(fitPoints(wide, { layer: "path", lastOnly: true })).toEqual([
      expect.objectContaining({ lat: 6, lon: 8 }),
    ])
  })
})

describe("view tuning", () => {
  it("reads zoom limits, padding and the fit choice", () => {
    expect(viewTuning({ minZoom: 2, maxZoom: 9, fitPadding: 10 })).toEqual({
      limits: { min: 2, max: 9 },
      paddingPercent: 10,
    })
    expect(viewTuning({})).toEqual({ limits: {} })
    expect(fitChoice({ fitLayer: "one", fitLastOnly: true })).toEqual({
      layer: "one",
      lastOnly: true,
    })
    expect(fitChoice({})).toEqual({ layer: undefined, lastOnly: false })
  })

  it("keeps every starting view inside the limits", () => {
    const size = [600, 300] as const
    const tuning = viewTuning({ minZoom: 3, maxZoom: 5 })
    const points = [{ lon: 10, lat: 50 }]
    expect(initialView(viewOptions({}), points, ...size, tuning).zoom).toBe(5)
    expect(
      initialView(viewOptions({ view: "world" }), points, ...size, tuning).zoom
    ).toBe(3)
    expect(
      initialView(
        viewOptions({ view: "coords", zoom: 12 }),
        points,
        ...size,
        tuning
      ).zoom
    ).toBe(5)
    expect(clampZoom(1, ...size, tuning.limits)).toBe(3)
  })

  it("pads the fit by a percent of the smaller side", () => {
    const size = [600, 300] as const
    const points = [
      { lon: -20, lat: 10 },
      { lon: 20, lat: 30 },
    ]
    const none = initialView(viewOptions({}), points, ...size, {
      paddingPercent: 0,
    })
    const tenth = initialView(viewOptions({}), points, ...size, {
      paddingPercent: 10,
    })
    expect(tenth).toEqual(fitBounds(points, ...size, { padding: 30 }))
    expect(none.zoom).toBeGreaterThan(tenth.zoom)
  })

  it("fits a region id to its bounds", () => {
    const options = viewOptions({ view: "europe" })
    expect(options.mode).toBe("europe")
    expect(initialView(options, [], 800, 500)).toEqual(
      fitBounds(regionCorners("europe"), 800, 500)
    )
  })
})

describe("symbolPath", () => {
  it("draws a different closed shape for every symbol", () => {
    const paths = markerSymbols.map((symbol) => symbolPath(symbol, 10))
    expect(new Set(paths).size).toBe(markerSymbols.length)
    for (const path of paths) expect(path.endsWith("Z")).toBe(true)
  })

  it("draws a square at its corners and a star inside its radius", () => {
    expect(symbolPath("square", 4)).toBe("M-4 -4L4 -4L4 4L-4 4Z")
    const numbers = symbolPath("star", 10).match(/-?\d+(\.\d+)?/g) ?? []
    expect(numbers).toHaveLength(20)
    for (const value of numbers) {
      expect(Math.abs(Number(value))).toBeLessThanOrEqual(10)
    }
  })
})
