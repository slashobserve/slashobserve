import { act, screen, waitFor } from "@testing-library/react"
import { useEffect, useState } from "react"
import {
  afterEach,
  beforeAll,
  beforeEach,
  describe,
  expect,
  it,
  vi,
} from "vitest"

import { DatasourceListProvider } from "@/datasources/queries"
import { loadSpecValidator } from "@/spec"
import { PanelHostContext } from "@/shell/side-panel"
import { renderWithProviders } from "@/test/render"

import {
  BlockEditorDock,
  blockSpec,
  editableBlock,
  newBlock,
  newQueryPanel,
  type BlockEditRequest,
} from "./block-editor"
import type { Block, Observation } from "./queries"

const author = {
  kind: "user",
  id: "u1",
  name: "Ada",
  origin: "web",
  client: null,
  run_id: null,
  label: "Ada",
}

const panel = {
  type: "timeseries",
  title: "Errors",
  datasource: "prom",
  queries: [{ ref: "A", query: "sum(rate(errors[5m]))" }],
}

function panelBlock(allowed: string[]): Block {
  return {
    id: "b1",
    observation_id: "o1",
    kind: "panel",
    state: "live",
    position: 2,
    width: "half",
    version: 3,
    body: { panel, from: "2026-11-12T08:00:00Z", to: "2026-11-12T09:00:00Z" },
    allowed_actions: allowed,
    created_by: author,
    created_at: "2026-11-12T10:00:00Z",
    updated_at: "2026-11-12T10:00:00Z",
  } as unknown as Block
}

const observation = {
  id: "o1",
  title: "Checkout slow",
  from: null,
  to: null,
} as unknown as Observation

const datasources = [
  { uid: "prom", name: "Prometheus", type: "prometheus", health: "ok" },
]

const saveWait = { timeout: 5000 }

type Sent = { method: string; path: string; body: unknown }
let sent: Sent[] = []

beforeAll(async () => {
  await loadSpecValidator()
}, 60_000)

beforeEach(() => {
  sent = []
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const url = new URL(request.url)
      const text = await request.text()
      sent.push({
        method: request.method,
        path: url.pathname,
        body: text === "" ? undefined : JSON.parse(text),
      })
      if (request.method === "PATCH" && url.pathname === "/api/v1/blocks/b1") {
        return Response.json({
          block: { ...panelBlock(["update"]), version: 4 },
        })
      }
      if (
        request.method === "POST" &&
        url.pathname === "/api/v1/observations/o1/blocks"
      ) {
        return Response.json(
          { block: { ...panelBlock(["update"]), id: "b2" } },
          { status: 201 }
        )
      }
      return new Response(null, { status: 599 })
    })
  )
})

afterEach(() => {
  vi.unstubAllGlobals()
  document.body.replaceChildren()
})

let setPageBlocks: (blocks: Block[]) => void = () => {}

function Page({
  request,
  initial,
  canEdit,
  onClose,
  onSaved,
}: {
  request: BlockEditRequest
  initial: Block[]
  canEdit: boolean
  onClose: () => void
  onSaved: (blockId: string) => void
}) {
  const [blocks, setBlocks] = useState(initial)
  useEffect(() => {
    setPageBlocks = setBlocks
  }, [])
  return (
    <BlockEditorDock
      hidden={false}
      observation={observation}
      blocks={blocks}
      request={request}
      canEdit={canEdit}
      onClose={onClose}
      onSaved={onSaved}
      replaceGuardRef={{ current: null }}
    />
  )
}

function renderDock(
  request: BlockEditRequest,
  blocks: Block[],
  canEdit: boolean
) {
  const host = document.createElement("div")
  document.body.append(host)
  const onSaved = vi.fn()
  const onClose = vi.fn()
  const rendered = renderWithProviders(
    <DatasourceListProvider value={datasources}>
      <PanelHostContext value={host}>
        <Page
          request={request}
          initial={blocks}
          canEdit={canEdit}
          onClose={onClose}
          onSaved={onSaved}
        />
      </PanelHostContext>
    </DatasourceListProvider>,
    { route: { path: "/", url: "/" } }
  )
  return { ...rendered, onSaved, onClose }
}

async function retitle(
  user: ReturnType<typeof renderDock>["user"],
  title: string
) {
  const field = await screen.findByLabelText("Title", {}, { timeout: 5000 })
  await user.clear(field)
  await user.type(field, title)
}

function writes() {
  return sent.filter(
    (request) =>
      request.path === "/api/v1/blocks/b1" ||
      request.path === "/api/v1/observations/o1/blocks"
  )
}

describe("BlockEditorDock", { timeout: 20_000 }, () => {
  it("saves an editor's change into the block it edits", async () => {
    const { user, onSaved } = renderDock(
      { kind: "edit", blockId: "b1" },
      [panelBlock(["update", "remove"])],
      true
    )
    expect(
      await screen.findByRole("heading", { name: "Edit Errors" })
    ).toBeTruthy()
    await retitle(user, "Checkout errors")
    await user.click(screen.getByRole("button", { name: "Save" }))
    await waitFor(() => expect(onSaved).toHaveBeenCalledWith("b1"), saveWait)
    const [write] = writes()
    expect(write?.method).toBe("PATCH")
    expect(write?.path).toBe("/api/v1/blocks/b1")
    const body = write!.body as {
      version: number
      body: { panel: Record<string, unknown>; from: string }
    }
    expect(body.version).toBe(3)
    expect(body.body.from).toBe("2026-11-12T08:00:00Z")
    expect(body.body.panel.title).toBe("Checkout errors")
    expect(body.body.panel).not.toHaveProperty("id")
    expect(body.body.panel).not.toHaveProperty("gridPos")
  })

  it("turns a commenter's edit into a requested block after the original", async () => {
    const { user, onSaved } = renderDock(
      { kind: "edit", blockId: "b1" },
      [panelBlock([])],
      false
    )
    expect(
      await screen.findByRole("heading", { name: "Suggest an edit to Errors" })
    ).toBeTruthy()
    await retitle(user, "Checkout errors")
    await user.click(screen.getByRole("button", { name: "Suggest edit" }))
    await waitFor(() => expect(onSaved).toHaveBeenCalledWith("b2"), saveWait)
    const [write] = writes()
    expect(write?.method).toBe("POST")
    expect(write?.path).toBe("/api/v1/observations/o1/blocks")
    const body = write!.body as {
      kind: string
      position: number
      width: string
      body: { panel: Record<string, unknown>; to: string }
    }
    expect(body.kind).toBe("panel")
    expect(body.position).toBe(3)
    expect(body.width).toBe("half")
    expect(body.body.to).toBe("2026-11-12T09:00:00Z")
    expect(body.body.panel.title).toBe("Checkout errors")
  })

  it("adds a new query as a panel block that follows the page", async () => {
    const { user, onSaved } = renderDock(
      {
        kind: "new",
        panel: {
          id: "panel",
          type: "timeseries",
          title: "New panel",
          datasource: "prom",
          gridPos: { x: 0, y: 0, w: 12, h: 8 },
          queries: [{ query: "up" }],
        },
      },
      [],
      true
    )
    expect(
      await screen.findByRole("heading", { name: "New query" })
    ).toBeTruthy()
    await retitle(user, "Targets up")
    await user.click(screen.getByRole("button", { name: "Add" }))
    await waitFor(() => expect(onSaved).toHaveBeenCalledWith("b2"), saveWait)
    const [write] = writes()
    expect(write?.path).toBe("/api/v1/observations/o1/blocks")
    const body = write!.body as {
      position?: number
      body: { panel: Record<string, unknown>; from?: string }
    }
    expect(body.position).toBeUndefined()
    expect(body.body.from).toBeUndefined()
    expect(body.body.panel.title).toBe("Targets up")
  })

  it("keeps a changed draft when someone deletes the block and adds it back as a new block", async () => {
    const { user, onSaved } = renderDock(
      { kind: "edit", blockId: "b1" },
      [panelBlock(["update"])],
      true
    )
    await retitle(user, "Checkout errors")
    act(() => setPageBlocks([]))

    expect(
      await screen.findByText(
        /Someone deleted this block\. Your draft stays here/
      )
    ).toBeTruthy()
    expect((screen.getByLabelText("Title") as HTMLInputElement).value).toBe(
      "Checkout errors"
    )
    await user.click(screen.getByRole("button", { name: "Add as a new block" }))
    await waitFor(() => expect(onSaved).toHaveBeenCalledWith("b2"), saveWait)
    const [write] = writes()
    expect(write?.method).toBe("POST")
    expect(write?.path).toBe("/api/v1/observations/o1/blocks")
    expect(write?.body).toMatchObject({
      kind: "panel",
      position: 2,
      width: "half",
      body: {
        from: "2026-11-12T08:00:00Z",
        panel: { title: "Checkout errors" },
      },
    })
  })

  it("asks before Escape throws away a changed draft", async () => {
    const { user, onClose } = renderDock(
      { kind: "edit", blockId: "b1" },
      [panelBlock(["update"])],
      true
    )
    await retitle(user, "Checkout errors")
    await user.keyboard("{Escape}")
    expect(
      await screen.findByRole("alertdialog", { name: "Discard your changes?" })
    ).toBeTruthy()
    await user.click(screen.getByRole("button", { name: "Discard" }))
    await waitFor(() => expect(onClose).toHaveBeenCalled(), saveWait)
    expect(writes()).toEqual([])
  })
})

describe("editableBlock", () => {
  const page = { from: "now-6h", to: "now" }

  it("opens a pinned panel block on its own window and variables", () => {
    const block = panelBlock(["update"])
    block.body = {
      ...block.body,
      variables: [{ name: "env", type: "custom", query: "prod,dev" }],
    }
    const editable = editableBlock(block, page)
    expect(editable).toMatchObject({
      blockId: "b1",
      version: 3,
      range: { from: "2026-11-12T08:00:00Z", to: "2026-11-12T09:00:00Z" },
      variables: [{ name: "env" }],
    })
    expect(editable?.panel).toMatchObject({
      id: "block",
      title: "Errors",
      gridPos: { x: 0, y: 0, w: 24, h: 8 },
    })
  })

  it("follows the page when the block has no window of its own", () => {
    const block = panelBlock(["update"])
    block.body = { panel }
    expect(editableBlock(block, page)?.range).toEqual({
      from: "now-6h",
      to: "now",
    })
  })

  it("opens nothing for a missing block, another kind or a row", () => {
    expect(editableBlock(undefined, page)).toBeNull()
    expect(
      editableBlock({ ...panelBlock([]), kind: "text" } as Block, page)
    ).toBeNull()
    expect(
      editableBlock(
        { ...panelBlock([]), body: { panel: { type: "row", title: "Group" } } },
        page
      )
    ).toBeNull()
  })
})

describe("newBlock", () => {
  it("starts a new query unsaved, with no variables, on the page's window", () => {
    const draft = newQueryPanel([{ uid: "prom" }])
    expect(draft).not.toBeNull()
    const editable = newBlock(draft!, { from: "now-6h", to: "now" })
    expect(editable).toMatchObject({
      blockId: null,
      version: 0,
      variables: [],
      range: { from: "now-6h", to: "now" },
      panel: { id: "block", datasource: "prom" },
    })
  })

  it("has no new query without a datasource", () => {
    expect(newQueryPanel([])).toBeNull()
  })
})

describe("blockSpec", () => {
  it("holds the one panel on the block's window and variables", () => {
    const editable = editableBlock(panelBlock(["update"]), {
      from: null,
      to: null,
    })!
    const spec = blockSpec("Checkout slow", editable)
    expect(spec.title).toBe("Checkout slow")
    expect(spec.time).toEqual(editable.range)
    expect(spec.panels).toEqual([editable.panel])
    expect(spec.variables).toEqual([])
  })
})
