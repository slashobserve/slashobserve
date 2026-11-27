import { describe, expect, it } from "vitest"

import type { Annotation } from "@/annotations/queries"
import type {
  Message,
  Thread,
  ThreadWithMessages,
} from "@/comments/thread-queries"
import type { Finding } from "@/findings/queries"
import type { VersionMeta } from "@/history/use-version-history"
import type { ProposalSummary } from "@/proposals/queries"
import type { Revision } from "@/revisions/queries"

import {
  buildTimeline,
  currentStep,
  filterTimeline,
  firstNewVersion,
  showsVersions,
  type ImportedDashboard,
  type RunningRequest,
  type TimelineEntry,
} from "./timeline-model"

const author = { kind: "user", id: "u1", name: "Ada" } as const
const actor = {
  ...author,
  origin: "web" as const,
  client: null,
  run_id: null,
  label: "Ada",
}

function at(minutes: number, ms = 0): string {
  return new Date(Date.UTC(2026, 0, 1, 0, minutes, 0, ms)).toISOString()
}

function message(id: string, threadId: string, createdAt: string): Message {
  return {
    id,
    thread_id: threadId,
    body: "text",
    actor,
    created_at: createdAt,
    updated_at: createdAt,
    edited_at: null,
    mentions: [],
    can_edit: false,
    can_delete: false,
    block_id: null,
    reply_to: null,
  }
}

function thread(
  overrides: Partial<Thread> & { id: string },
  replies = 0
): ThreadWithMessages {
  const createdAt = overrides.created_at ?? at(0)
  return {
    thread: {
      target: { kind: "dashboard", id: "d1" },
      dashboard_id: "d1",
      version: 1,
      panel_id: null,
      panel_present: true,
      status: "open",
      allowed_actions: ["reply", "resolve"],
      unread: false,
      created_at: createdAt,
      updated_at: createdAt,
      resolved_at: null,
      resolved_actor: null,
      resolved_by_version: null,
      revision: null,
      anchor: null,
      block_id: null,
      agent: null,
      agent_off: false,
      ...overrides,
    },
    messages: [
      message(overrides.id, overrides.id, createdAt),
      ...Array.from({ length: replies }, (_, index) =>
        message(`${overrides.id}-r${index}`, overrides.id, createdAt)
      ),
    ],
  }
}

function request(overrides: Partial<Revision> & { id: string }): Revision {
  return {
    dashboard_id: "d1",
    allowed_actions: [],
    intent: "apply",
    investigation: false,
    comment_id: null,
    panel_id: null,
    prompt: "do it",
    base_version: 1,
    status: "succeeded",
    state: "succeeded",
    result_kind: "none",
    attempts: 1,
    result_version: null,
    error: null,
    requested_by: author,
    origin: "web",
    origin_client: null,
    created_at: at(0),
    started_at: null,
    finished_at: null,
    failure_code: null,
    build: null,
    build_saved_at: null,
    continues_id: null,
    continued_by: null,
    retried_by: null,
    continue_note: null,
    redacted: false,
    ...overrides,
  }
}

function version(n: number, createdAt: string): VersionMeta {
  return {
    version: n,
    parent_version: n > 1 ? n - 1 : null,
    restored_from: null,
    summary: `v${n}`,
    author,
    created_at: createdAt,
    origin: "web",
    origin_client: null,
    agent_request_id: null,
    merged_proposal_id: null,
    actor: {
      kind: "user",
      id: "u1",
      name: "Ada",
      origin: "web",
      client: null,
      run_id: null,
      label: "Ada",
    },
  }
}

function imported(
  createdAt: string,
  unverified: { panelId: string; title: string; reason: string }[] = []
): ImportedDashboard {
  const record: ImportedDashboard["record"] = {
    id: "i1",
    dashboard_id: "d1",
    version: 1,
    source: "grafana",
    importer_version: 1,
    title: "Checkout",
    mapping: {},
    summary: {
      format: "classic",
      title: "Checkout",
      uid: "checkout",
      schema_version: 39,
      bytes: 1024,
      panels: 2,
      rows: 0,
      variables: 0,
    },
    datasources: [],
    report: {
      importer_version: 1,
      verified: true,
      counts: {
        total: 2,
        imported: 1,
        partial: 0,
        placeholder: 0,
        unverified: 1,
      },
      panels: [],
      notes: [],
    },
    created_by: author,
    actor: {
      kind: "user",
      id: "u1",
      name: "Ada",
      origin: "web",
      client: null,
      run_id: null,
      label: "Ada",
    },
    created_at: createdAt,
  }
  return { record, unverified }
}

function finding(id: string, createdAt: string): Finding {
  return {
    id,
    observation_id: id,
    agent_request_id: null,
    dashboard_id: "d1",
    folder_id: null,
    ask_on: null,
    title: "Checkout errors rose after the deploy",
    markdown: "The error rate doubled at 10:02.",
    evidence: [],
    from: at(0),
    to: at(10),
    as_of: null,
    annotation_id: null,
    created_by: { kind: "user", id: "u1", name: "Ada" },
    actor: {
      kind: "user",
      id: "u1",
      name: "Ada",
      origin: "web",
      client: null,
      run_id: null,
      label: "Ada",
    },
    created_at: createdAt,
    url: "",
    suggested_change: null,
    observe_worthy: false,
    restricted: false,
  }
}

function investigation(
  overrides: Partial<RunningRequest> & { id: string }
): RunningRequest {
  return {
    status: "running",
    dashboard_id: "d1",
    panel_id: null,
    created_at: at(0),
    started_at: at(0),
    ...overrides,
  }
}

function keys(entries: readonly TimelineEntry[]): string[] {
  return entries.map((entry) => entry.key)
}

const none = { threads: [], requests: [], versions: [] }

describe("buildTimeline", () => {
  it("orders every kind by time, oldest first", () => {
    const entries = buildTimeline({
      threads: [thread({ id: "c1", created_at: at(2) })],
      requests: [
        request({ id: "r1", created_at: at(3) }),
        request({
          id: "r2",
          created_at: at(5),
          status: "running",
          started_at: at(5),
        }),
      ],
      versions: [version(2, at(4)), version(1, at(1))],
    })
    expect(keys(entries)).toEqual([
      "version:1",
      "thread:c1",
      "request:r1",
      "version:2",
      "request:r2",
      "running:r2",
    ])
  })

  it("keeps every reply inside its thread entry", () => {
    const entries = buildTimeline({
      ...none,
      threads: [thread({ id: "c1", created_at: at(1) }, 1)],
    })
    expect(keys(entries)).toEqual(["thread:c1"])
    const [entry] = entries
    expect(entry?.kind === "thread" && entry.thread.messages).toHaveLength(2)
  })

  it("leaves out a thread with no loaded messages", () => {
    const empty = { ...thread({ id: "c1" }), messages: [] }
    expect(keys(buildTimeline({ ...none, threads: [empty] }))).toEqual([])
  })

  it("leaves every request of a loaded thread to that thread", () => {
    const entries = buildTimeline({
      ...none,
      threads: [thread({ id: "c1", created_at: at(1) })],
      requests: [
        request({
          id: "old",
          comment_id: "c1",
          status: "failed",
          created_at: at(2),
        }),
        request({
          id: "new",
          comment_id: "c1",
          status: "running",
          created_at: at(3),
          started_at: at(3),
        }),
      ],
    })
    expect(keys(entries)).toEqual(["thread:c1"])
  })

  it("shows a request whose thread is not loaded as a request entry", () => {
    const entries = buildTimeline({
      ...none,
      threads: [thread({ id: "c1", created_at: at(1) })],
      requests: [
        request({
          id: "r1",
          comment_id: "other",
          created_at: at(2),
        }),
      ],
    })
    expect(keys(entries)).toEqual(["thread:c1", "request:r1"])
  })

  it("says which version a file replaced on the version that replaced it", () => {
    const entries = buildTimeline({
      ...none,
      versions: [version(3, at(3)), version(2, at(2))],
      replaced: new Map([[3, 2]]),
    })
    expect(
      entries.map((entry) => entry.kind === "version" && entry.replaced)
    ).toEqual([null, 2])
  })

  it("leaves the observe request out and keeps its version", () => {
    const entries = buildTimeline({
      ...none,
      requests: [
        request({ id: "o1", base_version: null }),
        request({ id: "p1", created_at: at(2) }),
      ],
      versions: [version(1, at(1))],
    })
    expect(keys(entries)).toEqual(["version:1", "request:p1"])
  })

  it("shows a running entry for an active request outside a thread", () => {
    const entries = buildTimeline({
      ...none,
      threads: [thread({ id: "c1", created_at: at(1) })],
      requests: [
        request({
          id: "t1",
          comment_id: "c1",
          status: "queued",
          created_at: at(2),
        }),
        request({
          id: "o1",
          base_version: null,
          status: "running",
          created_at: at(3),
          started_at: at(4),
        }),
        request({ id: "done", status: "succeeded", created_at: at(5) }),
      ],
    })
    expect(keys(entries)).toEqual(["thread:c1", "running:o1", "request:done"])
  })

  it("puts a running entry one millisecond after its start", () => {
    const entries = buildTimeline({
      ...none,
      requests: [
        request({
          id: "r1",
          status: "running",
          created_at: at(1),
          started_at: at(2),
        }),
        request({ id: "r2", status: "queued", created_at: at(3) }),
      ],
    })
    const running = entries.filter((entry) => entry.kind === "running")
    expect(running.map((entry) => entry.at)).toEqual([at(2, 1), at(3, 1)])
  })

  it("sorts a running entry after its own request", () => {
    const entries = buildTimeline({
      ...none,
      requests: [
        request({
          id: "r1",
          status: "running",
          created_at: at(1),
          started_at: at(1),
        }),
      ],
      versions: [version(1, at(1, 1))],
    })
    expect(keys(entries)).toEqual(["request:r1", "running:r1", "version:1"])
  })

  it("orders ties by kind, then key", () => {
    const entries = buildTimeline({
      threads: [
        thread({ id: "b", created_at: at(1) }),
        thread({ id: "a", created_at: at(1) }),
      ],
      requests: [request({ id: "r1", created_at: at(1) })],
      versions: [version(2, at(1)), version(1, at(1))],
    })
    expect(keys(entries)).toEqual([
      "thread:a",
      "thread:b",
      "request:r1",
      "version:1",
      "version:2",
    ])
  })

  it("orders versions that share a time by number, not by text", () => {
    const entries = buildTimeline({
      ...none,
      versions: [version(10, at(1)), version(2, at(1)), version(9, at(1))],
    })
    expect(keys(entries)).toEqual(["version:2", "version:9", "version:10"])
  })

  it("sorts unparseable dates first and never throws", () => {
    const entries = buildTimeline({
      ...none,
      requests: [
        request({
          id: "bad",
          status: "running",
          created_at: "not a date",
          started_at: null,
        }),
        request({ id: "ok", created_at: at(1) }),
      ],
      versions: [version(1, "")],
    })
    expect(keys(entries)).toEqual([
      "request:bad",
      "running:bad",
      "version:1",
      "request:ok",
    ])
    expect(entries.find((entry) => entry.key === "running:bad")?.at).toBe(
      "not a date"
    )
  })

  it("puts the import after version 1 at the same time and before a later version", () => {
    const entries = buildTimeline({
      ...none,
      versions: [version(2, at(2)), version(1, at(1))],
      imported: imported(at(1)),
    })
    expect(keys(entries)).toEqual(["version:1", "import", "version:2"])
  })

  it("has no import entry without an import", () => {
    const versions = [version(1, at(1))]
    expect(keys(buildTimeline({ ...none, versions }))).toEqual(["version:1"])
    expect(keys(buildTimeline({ ...none, versions, imported: null }))).toEqual([
      "version:1",
    ])
  })

  it("carries the panel of each entry", () => {
    const entries = buildTimeline({
      threads: [thread({ id: "c1", panel_id: "p1" })],
      requests: [request({ id: "r1", panel_id: "p2", created_at: at(1) })],
      versions: [version(1, at(2))],
    })
    expect(entries.map((entry) => entry.panelId)).toEqual(["p1", "p2", null])
  })
})

describe("versions loaded across pages", () => {
  const firstPage = [version(4, at(4)), version(3, at(3))]
  const secondPage = [version(2, at(3)), version(1, at(1))]

  it("orders every loaded page together, oldest first", () => {
    const entries = buildTimeline({
      ...none,
      versions: [...firstPage, ...secondPage],
    })
    expect(keys(entries)).toEqual([
      "version:1",
      "version:2",
      "version:3",
      "version:4",
    ])
  })

  it("finds the topmost version that the last page added", () => {
    const entries = buildTimeline({
      ...none,
      threads: [thread({ id: "c1", created_at: at(2) })],
      versions: [...firstPage, ...secondPage],
    })
    expect(firstNewVersion(entries, new Set([3, 4]))).toBe(1)
  })

  it("finds no new version when nothing was added", () => {
    const entries = buildTimeline({ ...none, versions: firstPage })
    expect(firstNewVersion(entries, new Set([3, 4]))).toBeNull()
  })

  it.each([
    ["everything", null, true],
    ["changes", null, true],
    ["open", null, false],
    ["everything", "p1", false],
    ["changes", "p1", false],
  ] as const)(
    "shows versions for %s scoped to %s: %s",
    (show, panelId, expected) => {
      expect(showsVersions(show, panelId)).toBe(expected)
    }
  )
})

describe("filterTimeline", () => {
  const entries = buildTimeline({
    threads: [
      thread({ id: "open-p1", panel_id: "p1", created_at: at(1) }),
      thread({
        id: "done-p2",
        panel_id: "p2",
        status: "resolved",
        created_at: at(2),
      }),
      thread({ id: "open-all", created_at: at(3) }),
    ],
    requests: [
      request({ id: "r-p1", panel_id: "p1", created_at: at(4) }),
      request({
        id: "r-p2",
        panel_id: "p2",
        status: "running",
        created_at: at(5),
        started_at: at(5),
      }),
    ],
    versions: [version(1, at(6))],
  })

  it("keeps everything", () => {
    expect(filterTimeline(entries, "everything", null)).toEqual(entries)
  })

  it("keeps only open threads", () => {
    expect(keys(filterTimeline(entries, "open", null))).toEqual([
      "thread:open-p1",
      "thread:open-all",
    ])
  })

  it("keeps requests, running entries and versions as changes", () => {
    expect(keys(filterTimeline(entries, "changes", null))).toEqual([
      "request:r-p1",
      "request:r-p2",
      "running:r-p2",
      "version:1",
    ])
  })

  it("keeps a thread that has an agent request, whatever its status, as a change", () => {
    const withRequests = buildTimeline({
      ...none,
      threads: [
        thread({
          id: "asked",
          created_at: at(1),
          revision: { id: "r1", status: "succeeded" },
        }),
        thread({
          id: "failed",
          created_at: at(2),
          revision: { id: "r2", status: "failed" },
        }),
        thread({ id: "plain", created_at: at(3) }),
      ],
    })
    expect(keys(filterTimeline(withRequests, "changes", null))).toEqual([
      "thread:asked",
      "thread:failed",
    ])
  })

  it("drops other panels and every version when scoped to a panel", () => {
    expect(keys(filterTimeline(entries, "everything", "p1"))).toEqual([
      "thread:open-p1",
      "request:r-p1",
    ])
    expect(keys(filterTimeline(entries, "changes", "p2"))).toEqual([
      "request:r-p2",
      "running:r-p2",
    ])
  })

  describe("import entry", () => {
    const withImport = buildTimeline({
      ...none,
      versions: [version(1, at(1))],
      imported: imported(at(1), [
        { panelId: "p1", title: "Errors", reason: "no data" },
      ]),
    })

    it("shows only for everything", () => {
      expect(keys(filterTimeline(withImport, "everything", null))).toContain(
        "import"
      )
      expect(keys(filterTimeline(withImport, "open", null))).toEqual([])
      expect(keys(filterTimeline(withImport, "changes", null))).toEqual([
        "version:1",
      ])
    })

    it("stays under a panel scope only for an unverified panel", () => {
      expect(keys(filterTimeline(withImport, "everything", "p1"))).toEqual([
        "import",
      ])
      expect(keys(filterTimeline(withImport, "everything", "p2"))).toEqual([])
    })
  })

  it("combines the show filter with the panel scope", () => {
    expect(keys(filterTimeline(entries, "open", "p2"))).toEqual([])
    expect(keys(filterTimeline(entries, "open", "p1"))).toEqual([
      "thread:open-p1",
    ])
  })
})

describe("currentStep", () => {
  const step = (attempt: number, minutes: number) => ({
    attempt,
    at: at(minutes),
  })

  it("is zero without steps", () => {
    expect(currentStep([])).toBe(0)
  })

  it("counts every step of a single attempt", () => {
    expect(currentStep([step(1, 1), step(1, 2), step(1, 3)])).toBe(3)
  })

  it("ignores earlier failed attempts", () => {
    expect(currentStep([step(1, 1), step(1, 2), step(2, 3), step(2, 4)])).toBe(
      2
    )
  })

  it("starts over when a re-claimed run reuses the attempt number", () => {
    expect(currentStep([step(1, 5), step(1, 6), step(1, 1), step(1, 2)])).toBe(
      2
    )
  })

  it("keeps steps that share a timestamp together", () => {
    expect(currentStep([step(1, 1), step(1, 1), step(1, 1)])).toBe(3)
  })
})

describe("findings and investigations", () => {
  it("places each finding by when it was found, among the other entries", () => {
    const entries = buildTimeline({
      ...none,
      threads: [thread({ id: "c1", created_at: at(1) })],
      versions: [version(1, at(3))],
      findings: [finding("f2", at(4)), finding("f1", at(2))],
    })
    expect(keys(entries)).toEqual([
      "thread:c1",
      "finding:f1",
      "version:1",
      "finding:f2",
    ])
  })

  it("orders a finding after a version saved at the same moment", () => {
    const entries = buildTimeline({
      ...none,
      versions: [version(2, at(5))],
      findings: [finding("f1", at(5))],
    })
    expect(keys(entries)).toEqual(["version:2", "finding:f1"])
  })

  it("adds a running entry only for an investigation still queued or running", () => {
    const entries = buildTimeline({
      ...none,
      investigations: [
        investigation({ id: "queued", status: "queued", started_at: null }),
        investigation({ id: "running", panel_id: "p1" }),
        investigation({ id: "done", status: "succeeded" }),
        investigation({ id: "failed", status: "failed" }),
      ],
    })
    expect(
      entries.map((entry) => [entry.kind, entry.key, entry.panelId])
    ).toEqual([
      ["running", "running:queued", null],
      ["running", "running:running", "p1"],
    ])
  })

  it("keeps findings for everything, changes and a panel filter, not for open comments", () => {
    const entries = buildTimeline({
      ...none,
      threads: [thread({ id: "c1", created_at: at(1) })],
      findings: [finding("f1", at(2))],
    })
    expect(keys(filterTimeline(entries, "everything", null))).toEqual([
      "thread:c1",
      "finding:f1",
    ])
    expect(keys(filterTimeline(entries, "changes", null))).toEqual([
      "finding:f1",
    ])
    expect(keys(filterTimeline(entries, "open", null))).toEqual(["thread:c1"])
    expect(keys(filterTimeline(entries, "everything", "p1"))).toEqual([
      "finding:f1",
    ])
  })

  it("shows an open proposal and an annotation, and credits a merged proposal on its version", () => {
    const proposal = (
      id: string,
      state: ProposalSummary["state"],
      createdAt: string
    ) =>
      ({
        id,
        state,
        title: id,
        created_at: createdAt,
        actor: { ...actor, name: "Cleo", label: "Cleo" },
        closed_by: state === "merged" ? author : null,
      }) as ProposalSummary
    const annotation = {
      id: "a1",
      dashboard_id: "d1",
      panel_id: "p1",
      created_at: at(3),
    } as Annotation
    const merged = { ...version(2, at(2)), merged_proposal_id: "pm" }
    const entries = buildTimeline({
      ...none,
      versions: [version(1, at(0)), merged],
      proposals: [
        proposal("po", "open", at(1)),
        proposal("pm", "merged", at(1)),
      ],
      annotations: [annotation],
    })
    expect(keys(entries)).toEqual([
      "version:1",
      "proposal:po",
      "version:2",
      "annotation:a1",
    ])
    const credited = entries.find((entry) => entry.key === "version:2")
    expect(credited?.kind === "version" && credited.proposal?.id).toBe("pm")
    expect(keys(filterTimeline(entries, "open", null))).toEqual(["proposal:po"])
    expect(keys(filterTimeline(entries, "everything", "p1"))).toEqual([
      "proposal:po",
      "annotation:a1",
    ])
  })
})
