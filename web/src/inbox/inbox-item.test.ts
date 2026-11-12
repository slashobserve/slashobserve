import { describe, expect, test } from "vitest"

import {
  groupItems,
  itemHref,
  itemText,
  reasonLabel,
  unreadObservationIds,
  type InboxItem,
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
    ...over,
  }
}

function withData(data: Record<string, unknown>): InboxItem["data"] {
  return data as InboxItem["data"]
}

describe("itemText", () => {
  test.each([
    {
      name: "a proposal names a dashboard",
      over: {
        type: "proposal.opened",
        target: { kind: "proposal", id },
        data: withData({ title: "Tune the errors panel" }),
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
      name: "a reply on an observation",
      over: {
        type: "thread.replied",
        target: { kind: "thread", id },
        data: withData({ object: { kind: "observation", id: other } }),
      },
      text: { actor: "Ada", verb: "replied on", object: "an observation" },
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
        data: withData({ source: "observe", failure_code: "timeout" }),
      },
      text: { actor: "", verb: "Your run failed on", object: "a request" },
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

describe("itemHref", () => {
  test.each([
    {
      name: "a proposal opens its page",
      over: { target: { kind: "proposal", id } },
      href: `/proposals/${id}`,
    },
    {
      name: "an observation opens its page",
      over: { target: { kind: "observation", id } },
      href: `/o/${id}`,
    },
    {
      name: "a run opens its chat",
      over: { target: { kind: "run", id } },
      href: `/c/${id}`,
    },
    {
      name: "a dashboard opens with its slug",
      over: {
        target: { kind: "dashboard", id },
        data: withData({ title: "Checkout health" }),
      },
      href: `/d/${id}/checkout-health`,
    },
    {
      name: "a thread on a dashboard opens the dashboard",
      over: {
        target: { kind: "thread", id: other },
        data: withData({ object: { kind: "dashboard", id } }),
      },
      href: `/d/${id}`,
    },
    {
      name: "a thread on an observation opens the observation",
      over: {
        target: { kind: "thread", id: other },
        data: withData({ object: { kind: "observation", id } }),
      },
      href: `/o/${id}`,
    },
    {
      name: "a thread on a proposal opens the proposal",
      over: {
        target: { kind: "thread", id: other },
        data: withData({ object: { kind: "proposal", id } }),
      },
      href: `/proposals/${id}`,
    },
    {
      name: "a thread on an annotation opens its dashboard",
      over: {
        target: { kind: "thread", id: other },
        dashboard_id: id,
        data: withData({ object: { kind: "annotation", id: other } }),
      },
      href: `/d/${id}`,
    },
    {
      name: "a thread with no known object has no page",
      over: { target: { kind: "thread", id: other }, data: withData({}) },
      href: null,
    },
    {
      name: "a saved search has no page",
      over: { target: { kind: "saved_search", id } },
      href: null,
    },
  ] satisfies {
    name: string
    over: Partial<InboxItem>
    href: string | null
  }[])("$name", ({ over, href }) => {
    expect(itemHref(item(over))).toBe(href)
  })
})

describe("reasonLabel", () => {
  test.each([
    ["proposal", "Proposal"],
    ["requested_block", "Suggested block"],
    ["mention", "Mention"],
    ["reply", "Reply"],
    ["run_failed", "Run failed"],
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
