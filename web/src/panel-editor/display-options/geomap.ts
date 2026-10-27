import type { DisplayOption } from "./types"
import { fieldOption } from "./shared"

export const geomap: readonly DisplayOption[] = [
  {
    kind: "choice",
    key: "view",
    label: "View",
    options: [
      { value: "fit", label: "Fit to data" },
      { value: "world", label: "World" },
      { value: "coords", label: "Coordinates" },
    ],
  },
  {
    kind: "number",
    key: "lat",
    label: "Latitude",
    hint: "A number from -85 to 85.",
    rule: { min: -85, max: 85 },
  },
  {
    kind: "number",
    key: "lon",
    label: "Longitude",
    hint: "A number from -180 to 180.",
    rule: { min: -180, max: 180 },
  },
  {
    kind: "number",
    key: "zoom",
    label: "Zoom",
    hint: "A number from 1 to 18.",
    rule: { min: 1, max: 18 },
  },
  {
    kind: "choice",
    key: "location",
    label: "Location",
    options: [
      { value: "auto", label: "Auto" },
      { value: "coords", label: "Coordinates" },
      { value: "geohash", label: "Geohash" },
    ],
  },
  fieldOption("latField", "Latitude field", "Field named lat"),
  fieldOption("lonField", "Longitude field", "Field named lon"),
  fieldOption("geohashField", "Geohash field", "Field named geohash"),
  fieldOption("sizeField", "Size field", "First number field"),
  fieldOption("colorField", "Color field", "First number field"),
  {
    kind: "number",
    key: "markerSize",
    label: "Marker size",
    hint: "A whole number from 1 to 100.",
    rule: { min: 1, max: 100, integer: true },
  },
  {
    kind: "number",
    key: "markerSizeMax",
    label: "Largest marker size",
    hint: "A whole number from 1 to 100.",
    rule: { min: 1, max: 100, integer: true },
  },
]
