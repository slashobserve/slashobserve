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
  for (let i = a.length - 1; i >= 0; i--) {
    for (let j = b.length - 1; j >= 0; j--) {
      lcs[i][j] =
        a[i] === b[j]
          ? lcs[i + 1][j + 1] + 1
          : Math.max(lcs[i + 1][j], lcs[i][j + 1])
    }
  }

  const lines: DiffLine[] = []
  let i = 0
  let j = 0
  while (i < a.length && j < b.length) {
    if (a[i] === b[j]) {
      lines.push({ kind: "same", text: a[i] })
      i++
      j++
    } else if (lcs[i + 1][j] >= lcs[i][j + 1]) {
      lines.push({ kind: "del", text: a[i++] })
    } else {
      lines.push({ kind: "add", text: b[j++] })
    }
  }
  while (i < a.length) lines.push({ kind: "del", text: a[i++] })
  while (j < b.length) lines.push({ kind: "add", text: b[j++] })
  return lines
}
