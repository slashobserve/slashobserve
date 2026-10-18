function utf8Length(codePoint: number): number {
  if (codePoint < 0x80) return 1
  if (codePoint < 0x800) return 2
  if (codePoint < 0x10000) return 3
  return 4
}

export function byteRangeToIndices(
  text: string,
  start: number,
  end: number
): { from: number; to: number } {
  if (start === 0 && end === 0) return { from: 0, to: text.length }
  let from = text.length
  let to = text.length
  let foundFrom = false
  let bytes = 0
  let index = 0
  while (index < text.length) {
    if (!foundFrom && bytes >= start) {
      from = index
      foundFrom = true
    }
    if (bytes >= end) {
      to = index
      break
    }
    const codePoint = text.codePointAt(index) ?? 0
    bytes += utf8Length(codePoint)
    index += codePoint > 0xffff ? 2 : 1
  }
  if (to < from) to = from
  return { from, to }
}
