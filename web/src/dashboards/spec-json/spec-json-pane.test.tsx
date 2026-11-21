import { screen, waitFor } from "@testing-library/react"
import { afterEach, beforeEach, expect, it, vi } from "vitest"

import type { Dashboard, SpecValidator } from "@/spec"
import { renderWithProviders } from "@/test/render"

import { SpecJsonPane } from "./spec-json-pane"

const toasts = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }))
vi.mock("sonner", () => ({ toast: toasts }))

const acceptAll = ((value: unknown) => ({
  valid: true,
  spec: value,
})) as unknown as SpecValidator
vi.mock("@/lib/use-spec-validator", () => ({
  useSpecValidator: () => acceptAll,
}))

vi.mock("@/panel-editor/json-editor", () => ({
  JsonEditor: (props: {
    value: string
    onChange: (value: string) => void
    readOnly: boolean
    ariaLabel: string
  }) => (
    <textarea
      aria-label={props.ariaLabel}
      value={props.value}
      readOnly={props.readOnly}
      onChange={(event) => props.onChange(event.target.value)}
    />
  ),
}))

const dashboardId = "2ef9c24f-1628-4357-8198-b46e8762bfd8"
const spec = {
  specVersion: "0",
  title: "Shop",
  description: "before",
  panels: [],
} as unknown as Dashboard

function savedEnvelope(description: string) {
  return {
    dashboard: { id: dashboardId, title: "Shop" },
    version: { version: 5, spec: { ...spec, description } },
  }
}

beforeEach(() => {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const body = (await request.json()) as { spec: Dashboard }
      return new Response(
        JSON.stringify(savedEnvelope(body.spec.description ?? "")),
        { status: 201, headers: { "Content-Type": "application/json" } }
      )
    })
  )
})

afterEach(() => {
  vi.unstubAllGlobals()
  toasts.success.mockReset()
})

it("closes after a successful save without asking to discard the saved edits", async () => {
  const onClose = vi.fn()
  const { user } = renderWithProviders(
    <SpecJsonPane
      dashboardId={dashboardId}
      spec={spec}
      version={4}
      hidden={false}
      opener={() => null}
      onClose={onClose}
    />
  )
  const editor = await screen.findByRole("textbox", {
    name: "Dashboard Spec JSON",
  })
  await user.clear(editor)
  await user.click(editor)
  await user.paste(JSON.stringify({ ...spec, description: "after" }))
  await user.click(screen.getByRole("button", { name: "Save as version 5" }))

  await waitFor(() =>
    expect(toasts.success).toHaveBeenCalledWith("Saved version 5")
  )
  expect(
    screen.queryByRole("alertdialog", { name: "Discard your JSON edits?" })
  ).toBeNull()
  expect(onClose).toHaveBeenCalledTimes(1)
})
