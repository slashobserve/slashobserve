export function clientNameFrom(madeBy: string | null): string {
  return madeBy === null ? "Claude Code" : madeBy.replace(/^made by /, "")
}
