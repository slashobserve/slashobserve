export type HomeTab = "ask" | "observe" | "import"
export type HomeRouteSearch = { tab?: "observe" | "import" }

export function parseHomeTab(value: unknown): HomeTab {
  return value === "observe" || value === "import" ? value : "ask"
}

export function homeSearch(tab: HomeTab): HomeRouteSearch {
  return tab === "ask" ? {} : { tab }
}

export function homeRedirect(searchStr: string): string | null {
  const keys = [...new URLSearchParams(searchStr).keys()]
  return keys.every((key) => key === "tab") ? null : `/ask${searchStr}`
}
