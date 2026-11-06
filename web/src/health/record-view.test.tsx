import { renderHook } from "@testing-library/react"
import { afterEach, describe, expect, test, vi } from "vitest"

import { useRecordView } from "./record-view"

const first = "2f0f2c1e-6a53-4a35-8a0e-1d3b0d1f7a11"
const second = "7a1d2b3c-4d5e-4f60-8a9b-0c1d2e3f4a5b"

function stubFetch(status = 204) {
  const fetch = vi.fn<(request: Request) => Promise<Response>>(
    async () => new Response(null, { status })
  )
  vi.stubGlobal("fetch", fetch)
  return fetch
}

function postedPaths(fetch: ReturnType<typeof stubFetch>) {
  return fetch.mock.calls.map(([request]) => [
    request.method,
    new URL(request.url).pathname,
  ])
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe("useRecordView", () => {
  test("records one view per dashboard id while mounted", () => {
    const fetch = stubFetch()
    const { rerender } = renderHook(({ id }) => useRecordView(id), {
      initialProps: { id: first },
      reactStrictMode: true,
    })
    rerender({ id: first })
    rerender({ id: second })
    rerender({ id: second })
    expect(postedPaths(fetch)).toEqual([
      ["POST", `/api/v1/dashboards/${first}/views`],
      ["POST", `/api/v1/dashboards/${second}/views`],
    ])
  })
})
