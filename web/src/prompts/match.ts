import type { SavedPrompt } from "./queries"

export const pickerLimit = 50

const slashLine = /^\/([^\n]*)$/

export function pickerQuery(value: string): string | null {
  return slashLine.exec(value)?.[1] ?? null
}

export function matchPrompts(
  prompts: SavedPrompt[],
  query: string
): SavedPrompt[] {
  const needle = query.trim().toLocaleLowerCase()
  if (needle === "") return prompts
  const names = prompts.map((prompt) => prompt.name.toLocaleLowerCase())
  const bodies = prompts.map((prompt) => prompt.body.toLocaleLowerCase())
  const groups = [
    (index: number) => names[index]?.startsWith(needle),
    (index: number) => names[index]?.includes(needle),
    (index: number) => bodies[index]?.includes(needle),
  ]
  const taken = new Set<number>()
  const matched: SavedPrompt[] = []
  for (const test of groups) {
    prompts.forEach((prompt, index) => {
      if (taken.has(index) || !test(index)) return
      taken.add(index)
      matched.push(prompt)
    })
  }
  return matched
}

export function stepIndex(index: number, count: number, delta: 1 | -1): number {
  if (count === 0) return 0
  return (index + delta + count) % count
}
