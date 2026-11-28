export type DiffLine = { kind: "same" | "add" | "del"; text: string }

const maxLines = 400

export function lineDiff(before: string, after: string): DiffLine[] {
  const a = before === "" ? [] : before.split("\n")
  const b = after === "" ? [] : after.split("\n")
  if (a.length > maxLines || b.length > maxLines) {
    return [
      ...a.map((text): DiffLine => ({ kind: "del", text })),
      ...b.map((text): DiffLine => ({ kind: "add", text })),
    ]
  }

  const lcs = Array.from({ length: a.length + 1 }, () =>
    new Array<number>(b.length + 1).fill(0)
  )
  const cell = (i: number, j: number) => lcs[i]?.[j] ?? 0
  for (let i = a.length - 1; i >= 0; i--) {
    const row = lcs[i]
    if (row === undefined) continue
    for (let j = b.length - 1; j >= 0; j--) {
      row[j] =
        a[i] === b[j]
          ? cell(i + 1, j + 1) + 1
          : Math.max(cell(i + 1, j), cell(i, j + 1))
    }
  }

  const lines: DiffLine[] = []
  let i = 0
  let j = 0
  for (;;) {
    const x = a[i]
    const y = b[j]
    if (x === undefined || y === undefined) break
    if (x === y) {
      lines.push({ kind: "same", text: x })
      i++
      j++
    } else if (cell(i + 1, j) >= cell(i, j + 1)) {
      lines.push({ kind: "del", text: x })
      i++
    } else {
      lines.push({ kind: "add", text: y })
      j++
    }
  }
  for (const text of a.slice(i)) lines.push({ kind: "del", text })
  for (const text of b.slice(j)) lines.push({ kind: "add", text })
  return lines
}
