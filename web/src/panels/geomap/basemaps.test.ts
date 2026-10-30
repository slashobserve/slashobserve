import { describe, expect, it } from "vitest"

import {
  basemapOf,
  tileAccess,
  tileHosts,
  tileSource,
  tileUrl,
  unusableUrlReason,
  type Basemap,
} from "./basemaps"

function tilesOf(basemap: Basemap) {
  if (basemap.kind !== "tiles") throw new Error(`not tiles: ${basemap.kind}`)
  return basemap.source
}

const origin = "https://app.test"

describe("basemapOf", () => {
  it("draws the bundled outlines by default and for outline", () => {
    expect(basemapOf({}, "light")).toEqual({ kind: "outline" })
    expect(basemapOf({ basemap: "outline" }, "light")).toEqual({
      kind: "outline",
    })
  })

  it("draws nothing for none", () => {
    expect(basemapOf({ basemap: "none" }, "dark")).toEqual({ kind: "none" })
  })

  it.each([
    ["light", true, "light_all"],
    ["dark", true, "dark_all"],
    ["light", false, "light_nolabels"],
    ["dark", false, "dark_nolabels"],
  ] as const)("CARTO %s with labels %s uses %s", (theme, labels, style) => {
    const source = tilesOf(
      basemapOf(
        { basemap: "carto", basemapTheme: theme, basemapLabels: labels },
        "light"
      )
    )
    expect(tileUrl(source, { z: 3, x: 4, y: 2 })).toBe(
      `https://c.basemaps.cartocdn.com/${style}/3/4/2.png`
    )
  })

  it("lets an auto CARTO theme follow the app theme and labels default on", () => {
    const options = { basemap: "carto", basemapTheme: "auto" }
    expect(
      tileSource({ type: "carto", ...options }, "dark")?.template
    ).toContain("dark_all")
    expect(tilesOf(basemapOf(options, "light")).template).toContain("light_all")
  })

  it("picks the CARTO subdomain from the tile position", () => {
    const source = tilesOf(basemapOf({ basemap: "carto" }, "light"))
    const hosts = [0, 1, 2, 3, 4].map(
      (x) => new URL(tileUrl(source, { z: 5, x, y: 0 })).hostname
    )
    expect(hosts).toEqual([
      "a.basemaps.cartocdn.com",
      "b.basemaps.cartocdn.com",
      "c.basemaps.cartocdn.com",
      "d.basemaps.cartocdn.com",
      "a.basemaps.cartocdn.com",
    ])
  })

  it.each([
    ["streets", "World_Street_Map"],
    ["world-imagery", "World_Imagery"],
    ["world-physical", "World_Physical_Map"],
    ["topo", "World_Topo_Map"],
    ["usa-topo", "USA_Topo_Maps"],
    ["ocean", "Ocean/World_Ocean_Base"],
  ])(
    "Esri %s uses the %s service with row before column",
    (server, service) => {
      const source = tilesOf(
        basemapOf({ basemap: "esri", esriServer: server }, "light")
      )
      expect(tileUrl(source, { z: 3, x: 5, y: 2 })).toBe(
        `https://services.arcgisonline.com/ArcGIS/rest/services/${service}/MapServer/tile/3/2/5`
      )
    }
  )

  it("defaults Esri to streets", () => {
    const source = tilesOf(basemapOf({ basemap: "esri" }, "light"))
    expect(source.template).toContain("World_Street_Map")
    expect(source.attribution).toBe("Tiles © Esri")
  })

  it("fills OpenStreetMap tiles with column before row", () => {
    const source = tilesOf(basemapOf({ basemap: "osm" }, "light"))
    expect(tileUrl(source, { z: 2, x: 1, y: 3 })).toBe(
      "https://tile.openstreetmap.org/2/1/3.png"
    )
    expect(source.attribution).toBe("© OpenStreetMap contributors")
  })

  it("fills a custom template and turns {s} into a", () => {
    const source = tilesOf(
      basemapOf(
        {
          basemap: "xyz",
          basemapUrl: "https://{s}.tiles.example.com/{z}/{x}/{y}.png",
        },
        "light"
      )
    )
    expect(tileUrl(source, { z: 1, x: 0, y: 1 })).toBe(
      "https://a.tiles.example.com/1/0/1.png"
    )
  })

  it("attributes a custom basemap to its text, else to its host", () => {
    const url = "https://tiles.example.com/{z}/{x}/{y}.png"
    expect(
      tilesOf(
        basemapOf(
          { basemap: "xyz", basemapUrl: url, basemapAttribution: "Example" },
          "light"
        )
      ).attribution
    ).toBe("Example")
    expect(
      tilesOf(basemapOf({ basemap: "xyz", basemapUrl: url }, "light"))
        .attribution
    ).toBe("tiles.example.com")
  })

  it.each([
    ["missing", undefined],
    ["http", "http://tiles.example.com/{z}/{x}/{y}.png"],
    ["without placeholders", "https://tiles.example.com/tile.png"],
    ["without y", "https://tiles.example.com/{z}/{x}.png"],
  ])("refuses a custom basemap URL that is %s", (_, url) => {
    expect(basemapOf({ basemap: "xyz", basemapUrl: url }, "light")).toEqual({
      kind: "unusable",
      reason: unusableUrlReason,
    })
  })
})

describe("tileSource", () => {
  it("has no source for a layer type that is not a tile layer", () => {
    expect(tileSource({ type: "markers" }, "light")).toBeNull()
  })

  it("reads an overlay layer's own keys", () => {
    const source = tileSource(
      { type: "carto", theme: "dark", labels: false },
      "light"
    )
    expect(source?.template).toContain("dark_nolabels")
  })
})

describe("tileHosts", () => {
  it("lists every CARTO subdomain", () => {
    const source = tilesOf(basemapOf({ basemap: "carto" }, "light"))
    expect(tileHosts(source, origin).sort()).toEqual([
      "a.basemaps.cartocdn.com",
      "b.basemaps.cartocdn.com",
      "c.basemaps.cartocdn.com",
      "d.basemaps.cartocdn.com",
    ])
  })

  it("lists the one host of a single-host provider", () => {
    const source = tilesOf(basemapOf({ basemap: "osm" }, "light"))
    expect(tileHosts(source, origin)).toEqual(["tile.openstreetmap.org"])
  })
})

describe("tileAccess", () => {
  const carto = tilesOf(basemapOf({ basemap: "carto" }, "light"))
  const osm = tilesOf(basemapOf({ basemap: "osm" }, "light"))

  it("waits while the allowlist is loading", () => {
    expect(tileAccess(osm, undefined, origin)).toEqual({ state: "loading" })
  })

  it("allows a source whose hosts are all listed", () => {
    expect(tileAccess(carto, ["*.basemaps.cartocdn.com"], origin)).toEqual({
      state: "allowed",
    })
    expect(tileAccess(osm, ["tile.openstreetmap.org"], origin)).toEqual({
      state: "allowed",
    })
  })

  it("blocks a source with a host that is not listed", () => {
    expect(tileAccess(osm, ["example.com"], origin)).toEqual({
      state: "blocked",
      host: "tile.openstreetmap.org",
    })
    expect(tileAccess(osm, [], origin).state).toBe("blocked")
  })

  it("blocks CARTO when only one subdomain is listed", () => {
    expect(tileAccess(carto, ["a.basemaps.cartocdn.com"], origin).state).toBe(
      "blocked"
    )
  })
})
