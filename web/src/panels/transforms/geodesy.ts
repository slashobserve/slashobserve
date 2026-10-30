export type GeoPoint = { lat: number; lon: number }

const earthRadiusMeters = 6371008.8
const radians = Math.PI / 180

export function distanceMeters(from: GeoPoint, to: GeoPoint): number {
  const dLat = (to.lat - from.lat) * radians
  const dLon = (to.lon - from.lon) * radians
  const a =
    Math.sin(dLat / 2) ** 2 +
    Math.cos(from.lat * radians) *
      Math.cos(to.lat * radians) *
      Math.sin(dLon / 2) ** 2
  return 2 * earthRadiusMeters * Math.asin(Math.min(1, Math.sqrt(a)))
}

export function headingDegrees(from: GeoPoint, to: GeoPoint): number {
  const dLon = (to.lon - from.lon) * radians
  const fromLat = from.lat * radians
  const toLat = to.lat * radians
  const east = Math.sin(dLon) * Math.cos(toLat)
  const north =
    Math.cos(fromLat) * Math.sin(toLat) -
    Math.sin(fromLat) * Math.cos(toLat) * Math.cos(dLon)
  return (Math.atan2(east, north) / radians + 360) % 360
}

export function polygonAreaSquareMeters(corners: readonly GeoPoint[]): number {
  let sum = 0
  corners.forEach((a, index) => {
    const b = corners[(index + 1) % corners.length]
    sum +=
      (b.lon - a.lon) *
      radians *
      (2 + Math.sin(a.lat * radians) + Math.sin(b.lat * radians))
  })
  return Math.abs((sum * earthRadiusMeters ** 2) / 2)
}
