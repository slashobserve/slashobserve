export function lineAndColumn(text: string, offset: number) {
  const before = text.slice(0, Math.min(Math.max(offset, 0), text.length))
  const lines = before.split("\n")
  return {
    line: lines.length,
    column: [...lines[lines.length - 1]].length + 1,
  }
}
