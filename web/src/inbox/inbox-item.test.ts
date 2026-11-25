import { describe, expect, test } from "vitest"

import {
  itemExcerpt,
  itemHint,
  threadTarget,
  groupByTarget,
  groupItems,
  itemLink,
  itemText,
  reasonLabel,
  unreadObservationIds,
  type InboxItem,
  type ItemLink,
} from "./inbox-item"

const id = "3f2c8d1e-0b5a-4c7d-9e11-2a6b8c4d5e70"
const other = "a1b2c3d4-0b5a-4c7d-9e11-2a6b8c4d5e71"

function item(over: Partial<InboxItem> = {}): InboxItem {
  return {
    seq: 1,
    type: "dashboard.version_created",
    target: { kind: "dashboard", id },
    dashboard_id: id,
    folder_id: null,
    actor: {
      kind: "user",
      id: other,
      name: "Ada",
      origin: "web",
      client: null,
      run_id: null,
      label: "Ada",
    },
    data: {},
    created_at: "2026-11-11T10:00:00Z",
    reason: null,
    unread: false,
    target_title: null,
    excerpt: null,
    question: null,
    ...over,
  }
}

function withData(data: Record<string, unknown>): InboxItem["data"] {
  return data as InboxItem["data"]
}

describe("itemText", () => {
  test.each([
    {
      name: "a proposal names its title",
      over: {
        type: "proposal.opened",
        target: { kind: "proposal", id },
        data: withData({ title: "Tune the errors panel" }),
      },
      text: { actor: "Ada", verb: "proposed", object: "Tune the errors panel" },
    },
    {
      name: "a proposal with no title names a dashboard",
      over: {
        type: "proposal.opened",
        target: { kind: "proposal", id },
        data: withData({}),
      },
      text: {
        actor: "Ada",
        verb: "proposed a change to",
        object: "a dashboard",
      },
    },
    {
      name: "a requested block names its observation",
      over: {
        type: "observation.block_added",
        target: { kind: "observation", id },
        data: withData({ title: "Checkout latency", state: "requested" }),
      },
      text: {
        actor: "Ada",
        verb: "suggested a block on",
        object: "Checkout latency",
      },
    },
    {
      name: "a live block is an added block, not a suggestion",
      over: {
        type: "observation.block_added",
        target: { kind: "observation", id },
        data: withData({ title: "Checkout latency", state: "live" }),
      },
      text: {
        actor: "Ada",
        verb: "added a block to",
        object: "Checkout latency",
      },
    },
    {
      name: "a block without a title falls back to the kind word",
      over: {
        type: "observation.block_added",
        target: { kind: "observation", id },
        data: withData({ state: "requested" }),
      },
      text: {
        actor: "Ada",
        verb: "suggested a block on",
        object: "an observation",
      },
    },
    {
      name: "a mention names the object it is on",
      over: {
        type: "thread.mentioned",
        target: { kind: "thread", id },
        data: withData({ object: { kind: "dashboard", id: other } }),
      },
      text: { actor: "Ada", verb: "mentioned you on", object: "a dashboard" },
    },
    {
      name: "a mention names the target by its title",
      over: {
        type: "thread.mentioned",
        target: { kind: "thread", id },
        data: withData({ object: { kind: "dashboard", id: other } }),
        target_title: "Checkout latency",
      },
      text: {
        actor: "Ada",
        verb: "mentioned you on",
        object: "Checkout latency",
      },
    },
    {
      name: "a merged proposal names its title and the version it saved",
      over: {
        type: "proposal.merged",
        target: { kind: "proposal", id },
        data: withData({ title: "Add latency", revision: 2, version: 4 }),
      },
      text: {
        actor: "Ada",
        verb: "merged your proposal",
        object: "Add latency as version 4",
      },
    },
    {
      name: "a merged proposal with no saved version names only its title",
      over: {
        type: "proposal.merged",
        target: { kind: "proposal", id },
        data: withData({ title: "Add latency", revision: 2, version: null }),
      },
      text: {
        actor: "Ada",
        verb: "merged your proposal",
        object: "Add latency",
      },
    },
    {
      name: "a discarded proposal falls back to the target title",
      over: {
        type: "proposal.discarded",
        target: { kind: "proposal", id },
        target_title: "Add latency",
      },
      text: {
        actor: "Ada",
        verb: "discarded your proposal",
        object: "Add latency",
      },
    },
    {
      name: "a failed run names its dashboard by title",
      over: {
        type: "run.failed",
        target: { kind: "run", id },
        target_title: "Checkout",
      },
      text: { actor: "", verb: "Your run failed on", object: "Checkout" },
    },
    {
      name: "a reply on an observation",
      over: {
        type: "thread.replied",
        target: { kind: "thread", id },
        data: withData({ object: { kind: "observation", id: other } }),
      },
      text: { actor: "Ada", verb: "replied on", object: "an observation" },
    },
    {
      name: "the built-in agent's reply names the question it answered",
      over: {
        type: "thread.replied",
        target: { kind: "thread", id },
        actor: {
          kind: "user",
          id: other,
          name: "admin",
          origin: "web",
          client: null,
          run_id: "r1",
          label: "Observe Agent for admin",
        },
        question: "why did checkout slow down?",
        target_title: "Checkout",
        data: withData({ object: { kind: "dashboard", id: other } }),
      },
      text: {
        actor: "Observe Agent",
        verb: "answered",
        object: "why did checkout slow down?",
      },
    },
    {
      name: "a connected agent's reply keeps its own label with the question",
      over: {
        type: "thread.replied",
        target: { kind: "thread", id },
        actor: {
          kind: "user",
          id: other,
          name: "Ada",
          origin: "mcp",
          client: "Claude Code",
          run_id: null,
          label: "Ada (via Claude Code)",
        },
        question: "why did checkout slow down?",
      },
      text: {
        actor: "Ada (via Claude Code)",
        verb: "answered",
        object: "why did checkout slow down?",
      },
    },
    {
      name: "a connected service account's reply keeps its own name with the question",
      over: {
        type: "thread.replied",
        target: { kind: "thread", id },
        actor: {
          kind: "service_account",
          id: other,
          name: "ci-agent",
          origin: "mcp",
          client: null,
          run_id: null,
          label: "ci-agent (via MCP)",
        },
        question: "why did checkout slow down?",
      },
      text: {
        actor: "ci-agent (via MCP)",
        verb: "answered",
        object: "why did checkout slow down?",
      },
    },
    {
      name: "the built-in agent's reply with no question names where it answered",
      over: {
        type: "thread.replied",
        target: { kind: "thread", id },
        actor: {
          kind: "user",
          id: other,
          name: "admin",
          origin: "web",
          client: null,
          run_id: "r1",
          label: "Observe Agent for admin",
        },
        target_title: "Checkout",
        data: withData({ object: { kind: "dashboard", id: other } }),
      },
      text: { actor: "Observe Agent", verb: "answered on", object: "Checkout" },
    },
    {
      name: "a reply with no object uses the target kind",
      over: {
        type: "thread.replied",
        target: { kind: "thread", id },
        data: withData({}),
      },
      text: { actor: "Ada", verb: "replied on", object: "a thread" },
    },
    {
      name: "a failed run speaks to the reader",
      over: {
        type: "run.failed",
        target: { kind: "run", id },
        data: withData({ intent: "ask", failure_code: "timeout" }),
      },
      text: { actor: "", verb: "Your run failed on", object: "a request" },
    },
    {
      name: "a repair names its panels and dashboard",
      over: {
        type: "run.succeeded",
        reason: "repaired",
        target: { kind: "run", id },
        actor: { ...item().actor, label: "nightly-audit" },
        data: withData({
          result_version: 6,
          dashboard_title: "Node resources",
          panels_repaired: 2,
        }),
      },
      text: {
        actor: "nightly-audit",
        verb: "repaired 2 panels on",
        object: "Node resources",
      },
    },
    {
      name: "a repair of one panel reads in the singular",
      over: {
        type: "run.succeeded",
        reason: "repaired",
        target: { kind: "run", id },
        data: withData({
          result_version: 6,
          dashboard_title: "Node resources",
          panels_repaired: 1,
        }),
      },
      text: {
        actor: "Ada",
        verb: "repaired 1 panel on",
        object: "Node resources",
      },
    },
    {
      name: "a repair recorded without the title names its version",
      over: {
        type: "run.succeeded",
        reason: "repaired",
        target: { kind: "run", id },
        data: withData({ result_version: 6 }),
      },
      text: { actor: "", verb: "Your repair saved", object: "version 6" },
    },
    {
      name: "a repair recorded without the panel count names its version",
      over: {
        type: "run.succeeded",
        reason: "repaired",
        target: { kind: "run", id },
        data: withData({
          result_version: 6,
          dashboard_title: "Node resources",
        }),
      },
      text: { actor: "", verb: "Your repair saved", object: "version 6" },
    },
    {
      name: "a repair with no version or title names a version",
      over: {
        type: "run.succeeded",
        reason: "repaired",
        target: { kind: "run", id },
        data: withData({}),
      },
      text: { actor: "", verb: "Your repair saved", object: "a version" },
    },
    {
      name: "a succeeded run without the repaired reason keeps the fallback",
      over: {
        type: "run.succeeded",
        target: { kind: "run", id },
        data: withData({ result_version: 6 }),
      },
      text: { actor: "Ada", verb: "succeeded", object: "a request" },
    },
    {
      name: "any other type takes its verb from the type name",
      over: {
        type: "dashboard.deleted",
        target: { kind: "dashboard", id },
        data: withData({ title: "Checkout" }),
      },
      text: { actor: "Ada", verb: "deleted", object: "Checkout" },
    },
    {
      name: "a multi word action reads as words",
      over: {
        type: "dashboard.version_created",
        target: { kind: "dashboard", id },
        data: withData({}),
      },
      text: { actor: "Ada", verb: "version created", object: "a dashboard" },
    },
    {
      name: "the actor label is the byline",
      over: {
        type: "observation.resolved",
        target: { kind: "observation", id },
        actor: {
          ...item().actor,
          run_id: id,
          label: "Observe Agent for Ada",
        },
        data: withData({ title: "Slow pods" }),
      },
      text: {
        actor: "Observe Agent for Ada",
        verb: "resolved",
        object: "Slow pods",
      },
    },
  ] satisfies {
    name: string
    over: Partial<InboxItem>
    text: ReturnType<typeof itemText>
  }[])("$name", ({ over, text }) => {
    expect(itemText(item(over))).toEqual(text)
  })
})

describe("access requests", () => {
  const folder = "f0f0f0f0-0b5a-4c7d-9e11-2a6b8c4d5e72"

  test("a request names who asked and the object", () => {
    expect(
      itemText(
        item({
          type: "access_request.opened",
          target: { kind: "access_request", id: other },
          reason: "access_request",
          data: withData({
            object: { kind: "dashboard", id },
            title: "Checkout",
          }),
        })
      )
    ).toEqual({
      actor: "Ada",
      verb: "asked for comment access to",
      object: "Checkout",
    })
  })

  test("a request on an observation with no title names the kind", () => {
    expect(
      itemText(
        item({
          type: "access_request.opened",
          target: { kind: "access_request", id: other },
          data: withData({ object: { kind: "observation", id } }),
        })
      ).object
    ).toBe("an observation")
  })

  test("a request on a dashboard opens its Share pane", () => {
    expect(
      itemLink(
        item({
          type: "access_request.opened",
          target: { kind: "access_request", id: other },
          folder_id: folder,
          data: withData({ object: { kind: "dashboard", id } }),
        })
      )
    ).toEqual({ to: `/d/${id}`, search: { share: "1" } })
  })

  test("a request on an observation opens its folder's access settings", () => {
    expect(
      itemLink(
        item({
          type: "access_request.opened",
          target: { kind: "access_request", id: other },
          folder_id: folder,
          data: withData({ object: { kind: "observation", id } }),
        })
      )
    ).toEqual({ to: `/f/${folder}`, search: { settings: "1" } })
  })

  const inGeneral = (over: Partial<InboxItem> = {}) =>
    item({
      type: "access_request.opened",
      target: { kind: "access_request", id: other },
      folder_id: folder,
      data: withData({ object: { kind: "observation", id }, general: true }),
      ...over,
    })

  test("a request in General opens People, where the role is raised", () => {
    expect(itemLink(inGeneral())).toEqual({ to: "/people" })
    expect(
      itemLink(
        inGeneral({
          data: withData({ object: { kind: "dashboard", id }, general: true }),
        })
      )
    ).toEqual({ to: "/people" })
  })

  test("a service account's request in General opens API keys", () => {
    const key = inGeneral({
      actor: { ...item().actor, kind: "service_account", name: "ci-bot" },
    })
    expect(itemLink(key)).toEqual({ to: "/keys" })
    expect(itemHint(key)).toContain("Raise ci-bot's role on API keys")
  })

  test("a request in General says there is no list to change and what to do", () => {
    expect(itemHint(inGeneral())).toBe(
      "General is open to everyone by role and has no list to change. Raise Ada's role on People, or move it to a folder first."
    )
  })

  test("a request in another folder has no hint", () => {
    expect(
      itemHint(
        item({
          type: "access_request.opened",
          data: withData({ object: { kind: "dashboard", id } }),
        })
      )
    ).toBeNull()
  })
})

describe("itemLink", () => {
  test.each([
    {
      name: "a proposal opens its page",
      over: { target: { kind: "proposal", id } },
      link: { to: `/proposals/${id}` },
    },
    {
      name: "an observation opens its page",
      over: { target: { kind: "observation", id } },
      link: { to: `/o/${id}` },
    },
    {
      name: "a run opens its chat",
      over: { target: { kind: "run", id } },
      link: { to: `/c/${id}` },
    },
    {
      name: "a repair opens the compare of the version it saved",
      over: {
        type: "run.succeeded",
        reason: "repaired",
        target: { kind: "run", id: other },
        dashboard_id: id,
        data: withData({ result_version: 6 }),
      },
      link: { to: `/d/${id}/versions`, search: { from: 5, to: 6 } },
    },
    {
      name: "a repair that saved version 1 opens that version alone",
      over: {
        type: "run.succeeded",
        reason: "repaired",
        target: { kind: "run", id: other },
        dashboard_id: id,
        data: withData({ result_version: 1 }),
      },
      link: { to: `/d/${id}/versions`, search: { to: 1 } },
    },
    {
      name: "a repair with no version opens the dashboard",
      over: {
        type: "run.succeeded",
        reason: "repaired",
        target: { kind: "run", id: other },
        dashboard_id: id,
        data: withData({}),
      },
      link: { to: `/d/${id}` },
    },
    {
      name: "a repair with no dashboard has no page",
      over: {
        type: "run.succeeded",
        reason: "repaired",
        target: { kind: "run", id: other },
        dashboard_id: null,
        data: withData({ result_version: 6 }),
      },
      link: null,
    },
    {
      name: "a succeeded run without the repaired reason opens its chat",
      over: {
        type: "run.succeeded",
        target: { kind: "run", id },
        data: withData({ result_version: 6 }),
      },
      link: { to: `/c/${id}` },
    },
    {
      name: "a dashboard opens with its slug",
      over: {
        target: { kind: "dashboard", id },
        data: withData({ title: "Checkout health" }),
      },
      link: { to: `/d/${id}/checkout-health` },
    },
    {
      name: "a thread on a dashboard opens the dashboard on that thread",
      over: {
        target: { kind: "thread", id: other },
        data: withData({ object: { kind: "dashboard", id } }),
      },
      link: { to: `/d/${id}`, search: { thread: other } },
    },
    {
      name: "a thread on an observation opens the observation on that thread",
      over: {
        target: { kind: "thread", id: other },
        data: withData({ object: { kind: "observation", id } }),
      },
      link: { to: `/o/${id}`, search: { thread: other } },
    },
    {
      name: "a thread on a proposal opens the proposal on that thread",
      over: {
        target: { kind: "thread", id: other },
        data: withData({ object: { kind: "proposal", id } }),
      },
      link: { to: `/proposals/${id}`, search: { thread: other } },
    },
    {
      name: "a thread on an annotation opens its dashboard",
      over: {
        target: { kind: "thread", id: other },
        dashboard_id: id,
        data: withData({ object: { kind: "annotation", id: other } }),
      },
      link: { to: `/d/${id}` },
    },
    {
      name: "a thread with no known object has no page",
      over: { target: { kind: "thread", id: other }, data: withData({}) },
      link: null,
    },
    {
      name: "a saved search has no page",
      over: { target: { kind: "saved_search", id } },
      link: null,
    },
  ] satisfies {
    name: string
    over: Partial<InboxItem>
    link: ItemLink | null
  }[])("$name", ({ over, link }) => {
    expect(itemLink(item(over))).toEqual(link)
  })
})

describe("itemExcerpt", () => {
  test("is the excerpt, or null when there is none", () => {
    expect(itemExcerpt(item({ excerpt: "hello" }))).toBe("hello")
    expect(itemExcerpt(item({ excerpt: "" }))).toBeNull()
    expect(itemExcerpt(item({ excerpt: null }))).toBeNull()
  })
})

describe("threadTarget", () => {
  test("is the commented object of a thread item and nothing for other items", () => {
    const threaded = item({
      target: { kind: "thread", id },
      data: withData({ object: { kind: "observation", id: other } }),
    })
    expect(threadTarget(threaded)).toBe(`observation:${other}`)
    expect(threadTarget(item({}))).toBeNull()
  })
})

describe("reasonLabel", () => {
  test.each([
    ["proposal", "Proposal"],
    ["requested_block", "Suggested block"],
    ["mention", "Mention"],
    ["reply", "Reply"],
    ["run_failed", "Run failed"],
    ["repaired", "Repaired"],
    ["access_request", "Access request"],
  ] as const)("%s reads %s", (reason, label) => {
    expect(reasonLabel(reason)).toBe(label)
  })
})

describe("groupItems", () => {
  test("puts items with a reason under Needs you, keeping order in each group", () => {
    const groups = groupItems([
      item({ seq: 5 }),
      item({ seq: 4, reason: "mention" }),
      item({ seq: 3 }),
      item({ seq: 2, reason: "proposal" }),
    ])
    expect(
      groups.map((group) => [group.label, group.items.map((i) => i.seq)])
    ).toEqual([
      ["Needs you", [4, 2]],
      ["Workspace activity", [5, 3]],
    ])
  })

  test("leaves out a group that has no items", () => {
    expect(
      groupItems([item({ reason: "reply" })]).map((group) => group.label)
    ).toEqual(["Needs you"])
    expect(groupItems([item()]).map((group) => group.label)).toEqual([
      "Workspace activity",
    ])
    expect(groupItems([])).toEqual([])
  })
})

describe("groupByTarget", () => {
  test("puts the items about one dashboard or observation together under its title, newest group first", () => {
    const threadOn = (kind: "dashboard" | "observation", ref: string) =>
      ({
        type: "thread.mentioned",
        target: { kind: "thread", id: other },
        data: withData({ object: { kind, id: ref } }),
      }) satisfies Partial<InboxItem>
    const groups = groupByTarget([
      item({ seq: 6, ...threadOn("dashboard", id), target_title: "Checkout" }),
      item({
        seq: 5,
        ...threadOn("observation", other),
        target_title: "Slow search",
      }),
      item({ seq: 4, ...threadOn("dashboard", id), target_title: "Checkout" }),
      item({
        seq: 3,
        type: "proposal.opened",
        target: { kind: "proposal", id: other },
        target_title: "Raise the alert",
      }),
    ])
    expect(
      groups.map((group) => [group.label, group.items.map((i) => i.seq)])
    ).toEqual([
      ["Checkout", [6, 4]],
      ["Slow search", [5]],
      ["Raise the alert", [3]],
    ])
  })

  test("names a group with no title by what it is about and shows a mention as plain text", () => {
    const groups = groupByTarget([
      item({ seq: 2, target: { kind: "observation", id } }),
      item({
        seq: 1,
        target: { kind: "proposal", id: other },
        target_title: `Ask @[Ada](user:${id})`,
      }),
    ])
    expect(groups.map((group) => group.label)).toEqual([
      "an observation",
      "Ask @Ada",
    ])
  })
})

describe("unreadObservationIds", () => {
  test("collects observations with an unread item, directly or through a thread", () => {
    const ids = unreadObservationIds([
      item({
        target: { kind: "observation", id },
        unread: true,
        reason: "requested_block",
      }),
      item({
        target: { kind: "thread", id: other },
        data: withData({ object: { kind: "observation", id: other } }),
        unread: true,
        reason: "reply",
      }),
      item({
        target: { kind: "observation", id: "read-one" },
        unread: false,
        reason: "requested_block",
      }),
      item({ target: { kind: "proposal", id: "p" }, unread: true }),
    ])
    expect([...ids].sort()).toEqual([id, other].sort())
  })
})
