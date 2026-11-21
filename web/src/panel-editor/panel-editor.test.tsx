import { screen, waitFor } from "@testing-library/react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { DatasourceListProvider } from "@/datasources/queries"
import type { Dashboard, DataPanel } from "@/spec"
import { renderWithProviders } from "@/test/render"

import PanelEditor, {
  type BlockTarget,
  type DashboardTarget,
  type EditorTarget,
} from "./panel-editor"

const panel: DataPanel = {
  id: "errors",
  type: "timeseries",
  title: "Errors",
  datasource: "prom",
  gridPos: { x: 0, y: 0, w: 12, h: 8 },
  queries: [{ query: "sum(rate(errors[5m]))" }],
}

const spec: Dashboard = {
  specVersion: "0",
  title: "Checkout",
  time: { from: "now-1h", to: "now" },
  panels: [panel],
}

const datasources = [{ uid: "prom", type: "prometheus" }]
const saveWait = { timeout: 5000 }

beforeEach(() => {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => new Response(null, { status: 599 }))
  )
})

afterEach(() => {
  vi.unstubAllGlobals()
})

function renderEditor(target: EditorTarget) {
  const onSaved = vi.fn()
  const rendered = renderWithProviders(
    <DatasourceListProvider value={datasources}>
      <PanelEditor
        target={target}
        spec={spec}
        panel={panel}
        version={4}
        readOnly={false}
        view="editor"
        focusFirstQuery={false}
        onViewChange={() => {}}
        viewGuardRef={{ current: null }}
        timeRange={{ from: "now-1h", to: "now" }}
        selections={{}}
        onDirtyChange={() => {}}
        onSaved={onSaved}
        onOpenLatest={() => {}}
        onDiscard={() => {}}
      />
    </DatasourceListProvider>,
    { route: { path: "/", url: "/" } }
  )
  return { ...rendered, onSaved }
}

async function retitle(
  user: ReturnType<typeof renderEditor>["user"],
  title: string
) {
  const field = await screen.findByLabelText("Title", {}, { timeout: 5000 })
  await user.clear(field)
  await user.type(field, title)
}

function dashboardTarget(
  save: DashboardTarget["save"],
  propose = false
): DashboardTarget {
  return {
    kind: "dashboard",
    dashboardId: "d1",
    latestVersion: 4,
    adding: false,
    propose,
    deleted: false,
    save,
  }
}

describe("PanelEditor on a dashboard", { timeout: 20_000 }, () => {
  it("saves the panel as a patch on the version it was opened at", async () => {
    const save = vi.fn(async () => {})
    const { user, onSaved } = renderEditor(dashboardTarget(save))
    await retitle(user, "Checkout errors")
    await user.click(screen.getByRole("button", { name: "Save as version 5" }))
    await waitFor(() => expect(onSaved).toHaveBeenCalled(), saveWait)
    expect(save).toHaveBeenCalledTimes(1)
    expect(save).toHaveBeenCalledWith(
      expect.objectContaining({
        patch: {
          upsert: [
            expect.objectContaining({ id: "errors", title: "Checkout errors" }),
          ],
        },
        summary: expect.any(String),
        parent_version: 4,
        propose: false,
      })
    )
  })

  it("proposes the change when the person may only propose", async () => {
    const save = vi.fn(async () => {})
    const { user } = renderEditor(dashboardTarget(save, true))
    await retitle(user, "Checkout errors")
    await user.click(screen.getByRole("button", { name: "Propose change" }))
    await waitFor(
      () =>
        expect(save).toHaveBeenCalledWith(
          expect.objectContaining({ propose: true })
        ),
      saveWait
    )
  })
})

describe("PanelEditor on an observation block", { timeout: 20_000 }, () => {
  function blockTarget(save: BlockTarget["save"]): BlockTarget {
    return {
      kind: "block",
      observationId: "o1",
      blockId: "b1",
      saveLabel: "Suggest edit",
      outcome: "An editor keeps or discards your suggestion",
      save,
    }
  }

  it("hands the panel to the block's save under the block's label", async () => {
    const save = vi.fn(async () => {})
    const { user, onSaved } = renderEditor(blockTarget(save))
    await retitle(user, "Checkout errors")
    expect(
      screen.getByText(/An editor keeps or discards your suggestion/)
    ).toBeTruthy()
    expect(screen.queryByRole("button", { name: "Add a note" })).toBeNull()
    await user.click(screen.getByRole("button", { name: "Suggest edit" }))
    await waitFor(() => expect(onSaved).toHaveBeenCalled(), saveWait)
    expect(save).toHaveBeenCalledWith(
      expect.objectContaining({ id: "errors", title: "Checkout errors" })
    )
  })

  it("keeps the draft and says why when the block save fails", async () => {
    const save = vi.fn(async () => {
      throw new Error("offline")
    })
    const { user, onSaved } = renderEditor(blockTarget(save))
    await retitle(user, "Checkout errors")
    await user.click(screen.getByRole("button", { name: "Suggest edit" }))
    await waitFor(() => expect(save).toHaveBeenCalled(), saveWait)
    expect(onSaved).not.toHaveBeenCalled()
    expect((screen.getByLabelText("Title") as HTMLInputElement).value).toBe(
      "Checkout errors"
    )
  })
})
