import { hostAllowed, remoteHost } from "@/lib/remote-assets"

export type TileSource = {
  template: string
  subdomains: string
  attribution: string
}

export type TileCoordinate = { z: number; x: number; y: number }

export type AppTheme = "light" | "dark"

export type Basemap =
  | { kind: "outline" }
  | { kind: "none" }
  | { kind: "tiles"; source: TileSource }
  | { kind: "unusable"; reason: string }

export const outlineAttribution = "Natural Earth"
export const unusableUrlReason =
  "The custom basemap needs an https URL with {z}, {x} and {y}."

const cartoTemplate =
  "https://{s}.basemaps.cartocdn.com/{style}/{z}/{x}/{y}.png"
const esriTemplate =
  "https://services.arcgisonline.com/ArcGIS/rest/services/{service}/MapServer/tile/{z}/{y}/{x}"
const osmTemplate = "https://tile.openstreetmap.org/{z}/{x}/{y}.png"

const esriServices: Record<string, string> = {
  streets: "World_Street_Map",
  "world-imagery": "World_Imagery",
  "world-physical": "World_Physical_Map",
  topo: "World_Topo_Map",
  "usa-topo": "USA_Topo_Maps",
  ocean: "Ocean/World_Ocean_Base",
}

const cartoAttribution = "© OpenStreetMap contributors © CARTO"
const esriAttribution = "Tiles © Esri"
const osmAttribution = "© OpenStreetMap contributors"
const placeholderOrigin = "https://app.invalid"

export const tileLayerTypes: readonly string[] = ["carto", "esri", "osm", "xyz"]

function text(value: unknown): string | undefined {
  return typeof value === "string" && value !== "" ? value : undefined
}

function cartoSource(
  config: Record<string, unknown>,
  appTheme: AppTheme
): TileSource {
  const theme =
    config.theme === "light" || config.theme === "dark"
      ? config.theme
      : appTheme
  const suffix = config.labels === false ? "nolabels" : "all"
  return {
    template: cartoTemplate.replace("{style}", `${theme}_${suffix}`),
    subdomains: "abcd",
    attribution: cartoAttribution,
  }
}

function esriSource(config: Record<string, unknown>): TileSource {
  const service =
    esriServices[text(config.server) ?? "streets"] ?? esriServices.streets
  return {
    template: esriTemplate.replace("{service}", service),
    subdomains: "a",
    attribution: esriAttribution,
  }
}

function xyzSource(config: Record<string, unknown>): TileSource | null {
  const url = text(config.url)
  if (url === undefined || !url.startsWith("https://")) return null
  if (!["{z}", "{x}", "{y}"].every((part) => url.includes(part))) return null
  const host = remoteHost(url.replaceAll("{s}", "a"), placeholderOrigin)
  if (host === null) return null
  return {
    template: url,
    subdomains: "a",
    attribution: text(config.attribution) ?? host,
  }
}

export function tileSource(
  config: Record<string, unknown>,
  appTheme: AppTheme
): TileSource | null {
  switch (config.type) {
    case "carto":
      return cartoSource(config, appTheme)
    case "esri":
      return esriSource(config)
    case "osm":
      return {
        template: osmTemplate,
        subdomains: "a",
        attribution: osmAttribution,
      }
    case "xyz":
      return xyzSource(config)
    default:
      return null
  }
}

export function basemapOf(
  display: Record<string, unknown>,
  appTheme: AppTheme
): Basemap {
  const type = display.basemap
  if (type === "none") return { kind: "none" }
  if (typeof type !== "string" || !tileLayerTypes.includes(type)) {
    return { kind: "outline" }
  }
  const source = tileSource(
    {
      type,
      theme: display.basemapTheme,
      labels: display.basemapLabels,
      server: display.esriServer,
      url: display.basemapUrl,
      attribution: display.basemapAttribution,
    },
    appTheme
  )
  return source
    ? { kind: "tiles", source }
    : { kind: "unusable", reason: unusableUrlReason }
}

export function tileUrl(source: TileSource, tile: TileCoordinate): string {
  const subdomain =
    source.subdomains[(tile.x + tile.y) % source.subdomains.length]
  return source.template
    .replaceAll("{s}", subdomain)
    .replaceAll("{z}", String(tile.z))
    .replaceAll("{x}", String(tile.x))
    .replaceAll("{y}", String(tile.y))
}

export function tileHosts(source: TileSource, origin?: string): string[] {
  const hosts = new Set<string>()
  for (let shift = 0; shift < source.subdomains.length; shift++) {
    const host = remoteHost(tileUrl(source, { z: 0, x: shift, y: 0 }), origin)
    if (host !== null) hosts.add(host)
  }
  return [...hosts]
}

export type TileAccess =
  | { state: "loading" }
  | { state: "allowed" }
  | { state: "blocked"; host: string }

export function tileAccess(
  source: TileSource,
  patterns: readonly string[] | undefined,
  origin?: string
): TileAccess {
  if (patterns === undefined) return { state: "loading" }
  const host = tileHosts(source, origin).find(
    (candidate) => !hostAllowed(patterns, candidate)
  )
  return host === undefined ? { state: "allowed" } : { state: "blocked", host }
}
