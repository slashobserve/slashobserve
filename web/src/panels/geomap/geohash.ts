const alphabet = "0123456789bcdefghjkmnpqrstuvwxyz"
const maxLength = 12

export function decodeGeohash(
  text: string
): { lat: number; lon: number } | null {
  const hash = text.trim().toLowerCase()
  if (hash.length === 0 || hash.length > maxLength) return null
  const lat: [number, number] = [-90, 90]
  const lon: [number, number] = [-180, 180]
  let longitudeBit = true
  for (const char of hash) {
    const value = alphabet.indexOf(char)
    if (value < 0) return null
    for (let bit = 4; bit >= 0; bit--) {
      const range = longitudeBit ? lon : lat
      const middle = (range[0] + range[1]) / 2
      if ((value >> bit) & 1) range[0] = middle
      else range[1] = middle
      longitudeBit = !longitudeBit
    }
  }
  return { lat: (lat[0] + lat[1]) / 2, lon: (lon[0] + lon[1]) / 2 }
}
