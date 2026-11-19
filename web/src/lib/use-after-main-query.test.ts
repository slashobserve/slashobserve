import { renderHook } from "@testing-library/react"
import { describe, expect, it } from "vitest"

import { ApiError } from "@/api/errors"

import { useAfterMainQuery, type MainQueryState } from "./use-after-main-query"

const loading: MainQueryState = {
  isSuccess: false,
  isError: false,
  isPlaceholderData: false,
  error: null,
}
const answered: MainQueryState = { ...loading, isSuccess: true }
const placeholder: MainQueryState = { ...answered, isPlaceholderData: true }
const failed: MainQueryState = {
  ...loading,
  isError: true,
  error: new ApiError(400, "query_invalid", "bad query"),
}
const timedOutQuery: MainQueryState = {
  ...loading,
  isError: true,
  error: new ApiError(504, "query_timeout", "query timed out after 15s"),
}

function ready(main: MainQueryState, scope = "a") {
  return renderHook(({ state, key }) => useAfterMainQuery(state, key), {
    initialProps: { state: main, key: scope },
  })
}

describe("useAfterMainQuery", () => {
  it("holds the secondary requests while the main query loads", () => {
    expect(ready(loading).result.current).toEqual({ ready: false, slow: false })
  })

  it("starts them when the main query answers or fails", () => {
    expect(ready(answered).result.current.ready).toBe(true)
    expect(ready(failed).result.current.ready).toBe(true)
  })

  it("never starts them when the main query timed out", () => {
    expect(ready(timedOutQuery).result.current).toEqual({
      ready: false,
      slow: true,
    })
  })

  it("does not start them from the previous query's placeholder", () => {
    expect(ready(placeholder).result.current.ready).toBe(false)
  })

  it("stays started while the same scope pages through placeholders", () => {
    const hook = ready(answered)
    hook.rerender({ state: placeholder, key: "a" })
    expect(hook.result.current.ready).toBe(true)
    hook.rerender({ state: placeholder, key: "b" })
    expect(hook.result.current.ready).toBe(false)
  })

  it("starts after a retry of a timed out query answers", () => {
    const hook = ready(timedOutQuery)
    hook.rerender({ state: answered, key: "a" })
    expect(hook.result.current).toEqual({ ready: true, slow: false })
  })
})
