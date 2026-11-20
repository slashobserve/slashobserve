import { screen } from "@testing-library/react"
import { describe, expect, it } from "vitest"

import { renderWithProviders } from "@/test/render"

import { MessageText } from "./message-text"

type Props = Parameters<typeof MessageText>[0]
type Message = Props["message"]

const person: Message["actor"] = {
  kind: "user",
  id: "11111111-1111-4111-8111-111111111111",
  name: "Ada",
  origin: "web",
  client: null,
  run_id: null,
  label: "Ada",
}

function show(
  body: string,
  patch: Partial<Message> = {},
  folded = false,
  routed = false
) {
  const message: Message = {
    id: "m1",
    body,
    actor: person,
    mentions: [],
    can_edit: false,
    can_delete: false,
    ...patch,
  }
  return renderWithProviders(
    <MessageText
      message={message}
      people={[]}
      target={null}
      queryKey={["comments"]}
      folded={folded}
    />,
    routed ? { route: { path: "/", url: "/" } } : undefined
  )
}

describe("MessageText", () => {
  it("renders a person's Markdown", () => {
    const { container } = show("**bold** and `code` and\n\n- one\n- two")
    expect(container.querySelector("strong")?.textContent).toBe("bold")
    expect(container.querySelector("code")?.textContent).toBe("code")
    expect(container.querySelectorAll("li")).toHaveLength(2)
  })

  it("opens links safely and drops script links", () => {
    const { container } = show(
      "[docs](https://example.com/a) [bad](javascript:alert(1))"
    )
    const links = container.querySelectorAll("a")
    expect(links).toHaveLength(1)
    expect(links[0]?.getAttribute("href")).toBe("https://example.com/a")
    expect(links[0]?.getAttribute("rel")).toBe("noopener noreferrer")
    expect(container.textContent).toContain("bad")
  })

  it("never renders raw HTML", () => {
    const { container } = show(
      '<img src=x onerror="alert(1)"><script>alert(1)</script><b>hi</b>'
    )
    expect(container.querySelector("img")).toBeNull()
    expect(container.querySelector("script")).toBeNull()
    expect(container.querySelector("b")).toBeNull()
    expect(container.querySelector("[onerror]")).toBeNull()
  })

  it("keeps mention chips inside formatted text", () => {
    const token = "@[Grace](user:22222222-2222-4222-8222-222222222222)"
    const { container } = show(`**ask ${token} now**`, {
      mentions: [
        {
          kind: "user",
          id: "22222222-2222-4222-8222-222222222222",
          name: "Grace",
        },
      ],
    })
    const chip = container.querySelector("[data-mention='user']")
    expect(chip?.textContent).toBe("@Grace")
    expect(chip?.closest("strong")).not.toBeNull()
  })

  it("does not let typed marker characters forge a chip", () => {
    const { container } = show("0 plain")
    expect(container.querySelector("[data-mention]")).toBeNull()
    expect(container.textContent).toBe("0 plain")
  })

  it("never renders the agent's text as Markdown", () => {
    const run = { ...person, run_id: "33333333-3333-4333-8333-333333333333" }
    const { container } = show("**bold** [x](https://example.com)", {
      actor: run,
    })
    expect(container.querySelector("strong")).toBeNull()
    expect(container.querySelector("a")).toBeNull()
    expect(screen.getByText("**bold** [x](https://example.com)")).toBeVisible()
  })

  it("renders only the code spans of a run's reply and leaves the rest literal", () => {
    const run = { ...person, run_id: "33333333-3333-4333-8333-333333333333" }
    const { container } = show(
      "Wrote the answer: `checkout` is **slow** and `p99` too",
      { actor: run }
    )
    const codes = [...container.querySelectorAll("code")]
    expect(codes.map((code) => code.textContent)).toEqual(["checkout", "p99"])
    expect(container.querySelector("strong")).toBeNull()
    expect(container.textContent).toBe(
      "Wrote the answer: checkout is **slow** and p99 too"
    )
  })

  it("leaves an unpaired backtick in a run's reply as it is", () => {
    const run = { ...person, run_id: "33333333-3333-4333-8333-333333333333" }
    const { container } = show("Cut at `checkout and more...", { actor: run })
    expect(container.querySelector("code")).toBeNull()
    expect(container.textContent).toBe("Cut at `checkout and more...")
  })

  it("renders the code spans of an outside agent's comment as code", () => {
    const mcp = { ...person, origin: "mcp" as const, client: "Codex" }
    const { container } = show("see `checkout`", { actor: mcp })
    expect(container.querySelector("code")?.textContent).toBe("checkout")
    expect(container.textContent).toBe("see checkout")
  })

  it("shows the closing chat link of the agent's own run as a real link", async () => {
    const runId = "33333333-3333-4333-8333-333333333333"
    const run = { ...person, run_id: runId }
    const { container } = show(
      `Answered in the chat.\n\nFull answer (only Admins can open it): [the chat](/c/${runId})`,
      { actor: run },
      false,
      true
    )
    const link = await screen.findByRole("link", { name: "the chat" })
    expect(link.getAttribute("href")).toBe(`/c/${runId}`)
    expect(container.textContent).not.toContain("](")
  })

  it("leaves a chat link to someone else's run as plain text", () => {
    const run = { ...person, run_id: "33333333-3333-4333-8333-333333333333" }
    const other = "44444444-4444-4444-8444-444444444444"
    const { container } = show(`See [the chat](/c/${other})`, { actor: run })
    expect(container.querySelector("a")).toBeNull()
    expect(container.textContent).toContain(`[the chat](/c/${other})`)
  })

  it("renders an outside agent's comment as Markdown like a person's", () => {
    const mcp = { ...person, origin: "mcp" as const, client: "Codex" }
    const { container } = show("**bold** and `code`", { actor: mcp })
    expect(container.querySelector("strong")?.textContent).toBe("bold")
    expect(container.querySelector("code")?.textContent).toBe("code")
  })

  it("renders a service account's comment as Markdown like a person's", () => {
    const account = {
      ...person,
      kind: "service_account" as const,
      origin: "api" as const,
    }
    const { container } = show("**bold** and `code`", { actor: account })
    expect(container.querySelector("strong")?.textContent).toBe("bold")
    expect(container.querySelector("code")?.textContent).toBe("code")
  })

  it("never renders raw HTML from a service account", () => {
    const account = { ...person, kind: "service_account" as const }
    const { container } = show("<img src=x onerror=alert(1)><b>hi</b>", {
      actor: account,
    })
    expect(container.querySelector("img")).toBeNull()
    expect(container.querySelector("b")).toBeNull()
  })

  it("keeps a folded comment on one plain line", () => {
    const { container } = show("**bold**", {}, true)
    expect(container.querySelector("strong")).toBeNull()
  })
})
