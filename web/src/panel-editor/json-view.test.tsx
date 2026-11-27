import { act, fireEvent, render, screen } from "@testing-library/react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import type { Dashboard } from "@/spec"

import type { EditablePanel } from "./draft"
import { JsonView, type LeaveGuard } from "./json-view"

const evaluateJson = vi.hoisted(() => vi.fn())

vi.mock("@/lib/use-spec-validator", () => ({
  useSpecValidator: () => () => ({ valid: true }),
}))

vi.mock("./json-editor", () => ({
  JsonEditor: ({
    value,
    onChange,
  }: {
    value: string
    onChange: (value: string) => void
  }) => (
    <textarea
      aria-label="Panel JSON"
      value={value}
      onChange={(event) => onChange(event.target.value)}
    />
  ),
}))

vi.mock("./json-edit", async (importOriginal) => {
  const original = await importOriginal<typeof import("./json-edit")>()
  return { ...original, evaluateJson }
})

const draft = { id: "p1", type: "text", title: "One" } as EditablePanel
const spec = { panels: [draft] } as unknown as Dashboard

function mount() {
  const onValidityChange = vi.fn()
  const dispatch = vi.fn()
  let guard: LeaveGuard | undefined
  render(
    <JsonView
      spec={spec}
      draft={draft}
      dispatch={dispatch}
      readOnly={false}
      onValidityChange={onValidityChange}
      onLeaveRequest={(next) => {
        guard = next
      }}
    />
  )
  return { onValidityChange, dispatch, guard: () => guard }
}

beforeEach(() => {
  vi.useFakeTimers()
  evaluateJson.mockReset()
  evaluateJson.mockImplementation((text: string) => ({
    kind: "valid",
    panel: { ...draft, title: text },
    changesDraft: true,
  }))
})

afterEach(() => {
  vi.useRealTimers()
})

describe("JsonView", () => {
  it("validates once after typing stops", () => {
    const { dispatch, onValidityChange } = mount()
    const box = screen.getByLabelText("Panel JSON")
    fireEvent.change(box, { target: { value: "a" } })
    fireEvent.change(box, { target: { value: "ab" } })
    fireEvent.change(box, { target: { value: "abc" } })
    expect(evaluateJson).not.toHaveBeenCalled()
    expect(onValidityChange).toHaveBeenLastCalledWith(false)
    act(() => {
      vi.advanceTimersByTime(250)
    })
    expect(evaluateJson).toHaveBeenCalledTimes(1)
    expect(evaluateJson.mock.calls[0][0]).toBe("abc")
    expect(dispatch).toHaveBeenCalledTimes(1)
    expect(onValidityChange).toHaveBeenLastCalledWith(true)
  })

  it("validates a pending edit before a leave request is answered", () => {
    evaluateJson.mockReturnValue({ kind: "invalid", issues: [] })
    const { guard } = mount()
    fireEvent.change(screen.getByLabelText("Panel JSON"), {
      target: { value: "{" },
    })
    const leave = vi.fn()
    act(() => guard()?.(leave))
    expect(evaluateJson).toHaveBeenCalledTimes(1)
    expect(leave).not.toHaveBeenCalled()
  })

  it("lets a leave request through after a valid pending edit", () => {
    const { guard, dispatch } = mount()
    fireEvent.change(screen.getByLabelText("Panel JSON"), {
      target: { value: "ok" },
    })
    const leave = vi.fn()
    act(() => guard()?.(leave))
    expect(dispatch).toHaveBeenCalledTimes(1)
    expect(leave).toHaveBeenCalledTimes(1)
  })
})
