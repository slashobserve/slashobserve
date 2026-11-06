import { screen } from "@testing-library/react"
import type { UserEvent } from "@testing-library/user-event"
import { describe, expect, it, vi } from "vitest"

import { datasourceKeys } from "@/api/keys"
import { renderWithProviders, testQueryClient } from "@/test/render"

import { BuilderBar } from "./builder-bar"
import { LogsWorkspaceContext, type LogsWorkspace } from "./workspace"

function setup(draft: string) {
  const run = vi.fn()
  const setDraft = vi.fn()
  const setMode = vi.fn()
  const focusQuery = vi.fn()
  const workspace = {
    datasource: {
      uid: "ds",
      type: "elasticsearch",
      name: "ds",
      healthy: true,
      checked_at: "2026-11-07T12:00:00Z",
      dashboard_count: 0,
    },
    draft,
    setDraft,
    run,
    setMode,
    focusQuery,
  } as unknown as LogsWorkspace
  const queryClient = testQueryClient()
  queryClient.setQueryData(datasourceKeys.labels("ds", {}), {
    labels: ["log.level", "message", "host.name", "trace.id"],
    truncated: false,
  })
  queryClient.setQueryData(datasourceKeys.labelValues("ds", "log.level", {}), {
    values: ["error", "warn", "info"],
    truncated: false,
  })
  queryClient.setQueryData(datasourceKeys.labelValues("ds", "trace.id", {}), {
    values: [],
    truncated: false,
  })
  const view = renderWithProviders(
    <LogsWorkspaceContext value={workspace}>
      <BuilderBar />
    </LogsWorkspaceContext>,
    { queryClient }
  )
  return { ...view, run, setDraft, setMode, focusQuery }
}

async function pickField(user: UserEvent, field: string) {
  await user.type(screen.getByRole("combobox", { name: "Field" }), field)
  await user.click(await screen.findByRole("option", { name: field }))
}

describe("BuilderBar", () => {
  it("shows a chip for each clause with the KQL under them", () => {
    setup(
      "log.level : (error or warn) and message : payment and not host.name : canary-*"
    )
    expect(
      screen.getByRole("button", {
        name: "Edit filter log.level : error or warn",
      })
    ).toBeVisible()
    expect(
      screen.getByRole("button", { name: "Edit filter message : payment" })
    ).toBeVisible()
    expect(
      screen.getByRole("button", {
        name: "Edit filter not host.name : canary-*",
      })
    ).toBeVisible()
    expect(
      screen.getByText(
        "log.level : (error or warn) and message : payment and not host.name : canary-*"
      )
    ).toBeVisible()
    expect(screen.getByRole("button", { name: "Add a filter" })).toBeVisible()
  })

  it("removes a clause and runs what is left", async () => {
    const { user, setDraft, run } = setup("a:1 and b:2")
    await user.click(
      screen.getByRole("button", { name: "Remove filter a : 1" })
    )
    expect(setDraft).toHaveBeenCalledWith("b : 2")
    expect(run).toHaveBeenCalledWith("b : 2")
  })

  it("adds a filter from the field, operator and value and runs it", async () => {
    const { user, setDraft, run } = setup("message : payment")
    await user.click(screen.getByRole("button", { name: "Add a filter" }))
    await pickField(user, "log.level")
    await user.type(
      screen.getByRole("combobox", { name: "Value" }),
      "gateway{Enter}"
    )
    const text = "message : payment and log.level : gateway"
    expect(setDraft).toHaveBeenCalledWith(text)
    expect(run).toHaveBeenCalledWith(text)
  })

  it("picks a suggested value for the chosen field", async () => {
    const { user, run } = setup("")
    await user.click(screen.getByRole("button", { name: "Add a filter" }))
    await pickField(user, "log.level")
    await user.click(screen.getByRole("combobox", { name: "Value" }))
    await user.click(await screen.findByRole("option", { name: "error" }))
    await user.click(screen.getByRole("button", { name: "Add filter" }))
    expect(run).toHaveBeenCalledWith("log.level : error")
  })

  it("adds an exists filter without a value", async () => {
    const { user, run } = setup("")
    await user.click(screen.getByRole("button", { name: "Add a filter" }))
    await pickField(user, "trace.id")
    await user.click(screen.getByRole("combobox", { name: "Operator" }))
    await user.click(await screen.findByRole("option", { name: "exists" }))
    expect(screen.queryByRole("combobox", { name: "Value" })).toBeNull()
    await user.click(screen.getByRole("button", { name: "Add filter" }))
    expect(run).toHaveBeenCalledWith("trace.id : *")
  })

  it("collects several values for is one of", async () => {
    const { user, run } = setup("")
    await user.click(screen.getByRole("button", { name: "Add a filter" }))
    await pickField(user, "log.level")
    await user.click(screen.getByRole("combobox", { name: "Operator" }))
    await user.click(await screen.findByRole("option", { name: "is one of" }))
    const add = screen.getByRole("combobox", { name: "Add a value" })
    await user.type(add, "error{Enter}")
    await user.type(add, "warn{Enter}")
    expect(
      screen.getByRole("button", { name: "Remove value error" })
    ).toBeVisible()
    await user.click(screen.getByRole("button", { name: "Add filter" }))
    expect(run).toHaveBeenCalledWith("log.level : (error or warn)")
  })

  it("keeps the submit button off until the form is complete", async () => {
    const { user } = setup("")
    await user.click(screen.getByRole("button", { name: "Add a filter" }))
    expect(screen.getByRole("button", { name: "Add filter" })).toBeDisabled()
  })

  it("hands the text to the code editor from Edit as KQL", async () => {
    const { user, setMode, focusQuery } = setup("a:1")
    await user.click(screen.getByRole("button", { name: "Edit as KQL" }))
    expect(setMode).toHaveBeenCalledWith("code")
    expect(focusQuery).toHaveBeenCalled()
  })
})
