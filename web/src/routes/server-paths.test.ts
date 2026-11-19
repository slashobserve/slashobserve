import { describe, expect, test } from "vitest"

import routeTree from "../routeTree.gen.ts?raw"

const serverPaths = [
  "api",
  "oauth",
  ".well-known",
  "mcp",
  "healthz",
  "readyz",
  "metrics",
]

function webRoutePaths(): string[] {
  const block = /export interface FileRoutesByFullPath \{\n([\s\S]*?)\n\}/.exec(
    routeTree
  )
  if (block === null) throw new Error("FileRoutesByFullPath not found")
  return [...block[1].matchAll(/^\s*'([^']+)':/gm)].map((match) => match[1])
}

describe("web routes", () => {
  test("have routes to check", () => {
    expect(webRoutePaths()).toContain("/explore/metrics/")
  })

  test.each(serverPaths)(
    "none starts at the server's own path /%s",
    (serverPath) => {
      const clashing = webRoutePaths().filter(
        (route) => route.split("/")[1] === serverPath
      )
      expect(clashing).toEqual([])
    }
  )
})
