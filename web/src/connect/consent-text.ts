export function clientText(name: string | null): string {
  return name ?? "an unnamed MCP client"
}

export function clientTextAtStart(name: string | null): string {
  return name ?? "An unnamed MCP client"
}

export function labelText(personName: string, clientName: string | null) {
  return `${personName} (via ${clientName ?? "MCP"})`
}

export function returnUrl(raw: string): string | null {
  let url: URL
  try {
    url = new URL(raw)
  } catch {
    return null
  }
  return url.protocol === "https:" || url.protocol === "http:" ? raw : null
}
