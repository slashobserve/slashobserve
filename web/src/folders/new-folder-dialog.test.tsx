import { screen, waitFor } from "@testing-library/react"
import { afterEach, describe, expect, it, vi } from "vitest"

import { renderWithProviders } from "@/test/render"

import { NewFolderDialog } from "./new-folder-dialog"

afterEach(() => vi.unstubAllGlobals())

const created = {
  id: "f0f0f0f0-0b5a-4c7d-9e11-2a6b8c4d5e72",
  title: "Payments",
  general: false,
  personal: false,
}

function stubServer(status: number, body: unknown) {
  const bodies: unknown[] = []
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      bodies.push(await request.clone().json())
      return new Response(JSON.stringify(body), {
        status,
        headers: { "Content-Type": "application/json" },
      })
    })
  )
  return bodies
}

describe("NewFolderDialog", () => {
  it("creates the folder with the trimmed name and reports it", async () => {
    const bodies = stubServer(201, created)
    const onCreated = vi.fn()
    const onOpenChange = vi.fn()
    const { user } = renderWithProviders(
      <NewFolderDialog open onOpenChange={onOpenChange} onCreated={onCreated} />
    )
    await user.type(screen.getByRole("textbox", { name: "Name" }), " Payments ")
    await user.click(screen.getByRole("button", { name: "Create folder" }))
    await waitFor(() => expect(onCreated).toHaveBeenCalledTimes(1))
    expect(bodies).toEqual([{ title: "Payments" }])
    expect(onOpenChange).toHaveBeenCalledWith(false)
  })

  it("creates the folder when Enter is pressed in the name field", async () => {
    const bodies = stubServer(201, created)
    const onCreated = vi.fn()
    const { user } = renderWithProviders(
      <NewFolderDialog open onOpenChange={vi.fn()} onCreated={onCreated} />
    )
    await user.type(
      screen.getByRole("textbox", { name: "Name" }),
      "Payments{Enter}"
    )
    await waitFor(() => expect(onCreated).toHaveBeenCalledTimes(1))
    expect(bodies).toEqual([{ title: "Payments" }])
  })

  it("does nothing when Enter is pressed on an empty name", async () => {
    const bodies = stubServer(201, created)
    const { user } = renderWithProviders(
      <NewFolderDialog open onOpenChange={vi.fn()} onCreated={vi.fn()} />
    )
    await user.type(screen.getByRole("textbox", { name: "Name" }), "{Enter}")
    expect(bodies).toEqual([])
  })

  it("says when the name is taken and stays open", async () => {
    stubServer(409, { error: "exists", code: "folder_exists" })
    const onCreated = vi.fn()
    const { user } = renderWithProviders(
      <NewFolderDialog open onOpenChange={vi.fn()} onCreated={onCreated} />
    )
    await user.type(screen.getByRole("textbox", { name: "Name" }), "Payments")
    await user.click(screen.getByRole("button", { name: "Create folder" }))
    expect(
      await screen.findByText("A folder with this name already exists.")
    ).toBeVisible()
    expect(onCreated).not.toHaveBeenCalled()
  })
})
