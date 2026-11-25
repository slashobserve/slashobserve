export type GridLine<T> = {
  id: string
  rowId: string
  row: T
  detail: boolean
}

const detailSuffix = "#detail"

export function gridLines<T>(
  rows: readonly T[],
  rowId: (row: T) => string,
  open: ReadonlySet<string> | undefined
): GridLine<T>[] {
  return rows.flatMap((row) => {
    const id = rowId(row)
    const line = { id, rowId: id, row, detail: false }
    return open?.has(id)
      ? [line, { id: `${id}${detailSuffix}`, rowId: id, row, detail: true }]
      : [line]
  })
}
