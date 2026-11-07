import type { DatasourceStatus } from "./queries"

export type ExplorePage = "logs" | "traces"

const storageKey = "slashobserve-explore-datasource"

type Stored = Partial<Record<ExplorePage, string>>

function readStored(): Stored {
  try {
    const parsed: unknown = JSON.parse(localStorage.getItem(storageKey) ?? "")
    if (typeof parsed !== "object" || parsed === null) return {}
    const stored: Stored = {}
    for (const page of ["logs", "traces"] as const) {
      const uid = (parsed as Record<string, unknown>)[page]
      if (typeof uid === "string") stored[page] = uid
    }
    return stored
  } catch {
    return {}
  }
}

function preferredType(page: ExplorePage): string | undefined {
  return page === "traces" ? "tempo" : undefined
}

export function rememberedDatasource(
  page: ExplorePage,
  eligible: readonly DatasourceStatus[]
): string | undefined {
  const stored = readStored()[page]
  if (eligible.some((datasource) => datasource.uid === stored)) return stored
  const preferred = eligible.find(
    (datasource) => datasource.type === preferredType(page)
  )
  return (preferred ?? eligible[0])?.uid
}

export function rememberDatasource(page: ExplorePage, uid: string): void {
  try {
    localStorage.setItem(
      storageKey,
      JSON.stringify({ ...readStored(), [page]: uid })
    )
  } catch {
    return
  }
}
