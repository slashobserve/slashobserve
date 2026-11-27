import { screen, waitFor } from "@testing-library/react"
import type { UserEvent } from "@testing-library/user-event"
import { createRef } from "react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { serviceAccountKeys } from "@/api/keys"
import { PanelHostContext } from "@/shell/side-panel"
import { renderWithProviders, testQueryClient } from "@/test/render"

import { CreateKeyPanel } from "./create-key-panel"
import type { ServiceAccount } from "./queries"

function account(over: Partial<ServiceAccount> = {}): ServiceAccount {
  return {
    id: "a1",
    name: "ci-bot",
    description: "",
    created_by: "admin-1",
    created_at: "2026-11-01T00:00:00Z",
    role: "editor",
    provisioned: false,
    takes_runs: false,
    presence: "offline",
    last_seen_at: null,
    ...over,
  }
}

type Call = { method: string; url: string; body: unknown }

function serve(current: ServiceAccount, keyFailures = 0) {
  const calls: Call[] = []
  let failuresLeft = keyFailures
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const body =
        request.method === "GET" ? null : ((await request.json()) as unknown)
      calls.push({ method: request.method, url: request.url, body })
      const json = (value: unknown, status = 200) =>
        new Response(JSON.stringify(value), {
          status,
          headers: { "Content-Type": "application/json" },
        })
      if (request.method === "PATCH") {
        return json({ ...current, ...(body as object) })
      }
      if (request.method === "POST" && request.url.endsWith("/keys")) {
        if (failuresLeft > 0) {
          failuresLeft -= 1
          return json({ code: "internal", message: "key store is down" }, 500)
        }
        return json(
          { token: "sok_secret", key: { id: "k1", name: "laptop" } },
          201
        )
      }
      if (
        request.method === "POST" &&
        request.url.endsWith("/service-accounts")
      ) {
        return json({ ...current, id: "a2", ...(body as object) }, 201)
      }
      return json({})
    })
  )
  return calls
}

function renderPanel(current: ServiceAccount, onClose = () => undefined) {
  const queryClient = testQueryClient()
  queryClient.setQueryData(serviceAccountKeys.list(), { items: [current] })
  const host = document.createElement("div")
  document.body.append(host)
  return renderWithProviders(
    <PanelHostContext.Provider value={host}>
      <CreateKeyPanel
        onClose={onClose}
        nameInput={createRef()}
        copyButton={createRef()}
      />
    </PanelHostContext.Provider>,
    { queryClient, route: { path: "/", url: "/" } }
  )
}

async function chooseExistingAccount(user: UserEvent, name: string) {
  await user.click(await screen.findByLabelText("Service account"))
  await user.click(await screen.findByRole("option", { name }))
}

beforeEach(() => {
  document.body.replaceChildren()
})

afterEach(() => {
  vi.unstubAllGlobals()
})

describe("CreateKeyPanel takes agent runs", () => {
  it("starts on a new service account even when accounts exist", async () => {
    serve(account())
    renderPanel(account())
    expect(await screen.findByLabelText("Service account name")).toBeVisible()
    expect(screen.getByLabelText("Service account")).toHaveTextContent(
      "New service account"
    )
  })

  it("shows the switch off for an account that does not take runs", async () => {
    serve(account())
    const { user } = renderPanel(account())
    await chooseExistingAccount(user, "ci-bot")
    const toggle = await screen.findByRole("switch", {
      name: "Takes agent runs",
    })
    expect(toggle).not.toBeChecked()
    expect(toggle).not.toHaveAttribute("aria-disabled", "true")
  })

  it("sends takes_runs with the new key and nothing else when only the switch changed", async () => {
    const calls = serve(account())
    const { user } = renderPanel(account())
    await chooseExistingAccount(user, "ci-bot")
    await user.click(
      await screen.findByRole("switch", { name: "Takes agent runs" })
    )
    await user.type(screen.getByLabelText("Name"), "laptop")
    await user.click(screen.getByRole("button", { name: "Create key" }))
    await waitFor(() =>
      expect(calls.some((call) => call.url.endsWith("/keys"))).toBe(true)
    )
    const patches = calls.filter((call) => call.method === "PATCH")
    expect(patches).toHaveLength(1)
    expect(patches[0].url).toMatch(/\/service-accounts\/a1$/)
    expect(patches[0].body).toEqual({ takes_runs: true })
  })

  it("turns an account that takes runs off", async () => {
    const current = account({ takes_runs: true })
    const calls = serve(current)
    const { user } = renderPanel(current)
    await chooseExistingAccount(user, "ci-bot")
    const toggle = await screen.findByRole("switch", {
      name: "Takes agent runs",
    })
    expect(toggle).toBeChecked()
    await user.click(toggle)
    await user.type(screen.getByLabelText("Name"), "laptop")
    await user.click(screen.getByRole("button", { name: "Create key" }))
    await waitFor(() =>
      expect(calls.some((call) => call.url.endsWith("/keys"))).toBe(true)
    )
    expect(calls.find((call) => call.method === "PATCH")?.body).toEqual({
      takes_runs: false,
    })
  })

  it("sends no update when neither role nor switch changed", async () => {
    const calls = serve(account())
    const { user } = renderPanel(account())
    await chooseExistingAccount(user, "ci-bot")
    await screen.findByRole("switch", { name: "Takes agent runs" })
    await user.type(screen.getByLabelText("Name"), "laptop")
    await user.click(screen.getByRole("button", { name: "Create key" }))
    await waitFor(() =>
      expect(calls.some((call) => call.url.endsWith("/keys"))).toBe(true)
    )
    expect(calls.some((call) => call.method === "PATCH")).toBe(false)
  })

  it("locks the switch for an account from the service-accounts file", async () => {
    const current = account({ provisioned: true, takes_runs: true })
    serve(current)
    const { user } = renderPanel(current)
    await chooseExistingAccount(user, "ci-bot")
    expect(
      await screen.findByRole("switch", { name: "Takes agent runs" })
    ).toHaveAttribute("aria-disabled", "true")
    expect(
      screen.getByText(/Set in the service-accounts file/)
    ).toBeInTheDocument()
  })
})

describe("CreateKeyPanel role", () => {
  it("keeps the panel open and creates the key with the role picked by mouse", async () => {
    const calls = serve(account())
    const onClose = vi.fn()
    const { user } = renderPanel(account(), onClose)
    await user.type(await screen.findByLabelText("Name"), "laptop")
    await user.type(screen.getByLabelText("Service account name"), "new-bot")
    await user.click(screen.getByLabelText("Role"))
    await user.click(await screen.findByRole("option", { name: /^Viewer/ }))
    expect(screen.getByLabelText("Role")).toHaveTextContent("Viewer")
    await user.click(screen.getByRole("button", { name: "Create key" }))
    expect(await screen.findByText("sok_secret")).toBeVisible()
    expect(onClose).not.toHaveBeenCalled()
    const accountCall = calls.find((call) =>
      call.url.endsWith("/service-accounts")
    )
    expect(accountCall?.body).toMatchObject({ name: "new-bot", role: "viewer" })
    expect(calls.some((call) => call.url.endsWith("/keys"))).toBe(true)
  })

  it("picks the role with the keyboard", async () => {
    serve(account())
    const { user } = renderPanel(account())
    await screen.findByLabelText("Service account name")
    screen.getByLabelText("Role").focus()
    await user.keyboard("{ArrowDown}{ArrowDown}{ArrowDown}{Enter}")
    await waitFor(() =>
      expect(screen.getByLabelText("Role")).toHaveTextContent("Viewer")
    )
    expect(screen.queryByRole("listbox")).not.toBeInTheDocument()
  })

  it("keeps the panel open when the last expiry is picked by mouse", async () => {
    serve(account())
    const onClose = vi.fn()
    const { user } = renderPanel(account(), onClose)
    await user.click(await screen.findByLabelText("Expires"))
    const options = await screen.findAllByRole("option")
    await user.click(options[options.length - 1])
    expect(onClose).not.toHaveBeenCalled()
  })
})

describe("CreateKeyPanel after the account was created and the key was not", () => {
  it("says the account exists and issues the key again without a second account", async () => {
    const calls = serve(account(), 1)
    const { user } = renderPanel(account())
    await user.type(await screen.findByLabelText("Name"), "laptop")
    await user.type(screen.getByLabelText("Service account name"), "new-bot")
    await user.click(screen.getByRole("button", { name: "Create key" }))

    expect(
      await screen.findByText(
        /The service account new-bot was created, but it has no key yet\./
      )
    ).toBeVisible()
    expect(screen.getByLabelText("Service account")).toHaveTextContent(
      "new-bot"
    )
    await user.click(
      screen.getByRole("button", { name: "Issue the key again" })
    )

    expect(await screen.findByText("sok_secret")).toBeVisible()
    const accountPosts = calls.filter(
      (call) => call.method === "POST" && call.url.endsWith("/service-accounts")
    )
    expect(accountPosts).toHaveLength(1)
    const keyPosts = calls.filter((call) => call.url.endsWith("/keys"))
    expect(keyPosts).toHaveLength(2)
    expect(keyPosts[1].url).toMatch(/\/service-accounts\/a2\/keys$/)
  })

  it("keeps the plain form when an existing account's key fails", async () => {
    serve(account(), 1)
    const { user } = renderPanel(account())
    await chooseExistingAccount(user, "ci-bot")
    await user.type(screen.getByLabelText("Name"), "laptop")
    await user.click(screen.getByRole("button", { name: "Create key" }))
    expect(await screen.findByRole("alert")).toBeVisible()
    expect(screen.getByRole("button", { name: "Create key" })).toBeVisible()
    expect(screen.queryByText(/was created, but/)).not.toBeInTheDocument()
  })
})
