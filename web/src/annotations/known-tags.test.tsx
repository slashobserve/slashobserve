import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { act, renderHook } from "@testing-library/react"
import type { ReactNode } from "react"
import { describe, expect, it } from "vitest"

import { annotationKeys } from "@/api/keys"

import { useKnownAnnotationTags } from "./queries"

function annotation(tags: string[]) {
  return { tags }
}

describe("useKnownAnnotationTags", () => {
  it("lists the distinct tags of every cached range of the dashboard", () => {
    const queryClient = new QueryClient()
    queryClient.setQueryData(annotationKeys.range("d1", "now-1h", "now"), [
      annotation(["deploy", "alert"]),
    ])
    queryClient.setQueryData(annotationKeys.range("d1", "now-6h", "now"), [
      annotation(["deploy", "release"]),
    ])
    queryClient.setQueryData(annotationKeys.range("d2", "now-1h", "now"), [
      annotation(["other"]),
    ])
    const wrapper = ({ children }: { children: ReactNode }) => (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    )
    const { result } = renderHook(() => useKnownAnnotationTags("d1"), {
      wrapper,
    })
    expect(result.current).toEqual(["alert", "deploy", "release"])

    act(() => {
      queryClient.setQueryData(annotationKeys.range("d1", "now-1h", "now"), [
        annotation(["zeta"]),
      ])
    })
    expect(result.current).toEqual(["deploy", "release", "zeta"])
  })

  it("is empty before any range has loaded", () => {
    const queryClient = new QueryClient()
    const wrapper = ({ children }: { children: ReactNode }) => (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    )
    const { result } = renderHook(() => useKnownAnnotationTags("d1"), {
      wrapper,
    })
    expect(result.current).toEqual([])
  })
})
