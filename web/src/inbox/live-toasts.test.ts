import { describe, expect, it } from "vitest"

import type { InboxItem } from "./inbox-item"
import { gateLiveToasts, planLiveToasts } from "./live-toasts"

function item(seq: number, reason: InboxItem["reason"] = "mention"): InboxItem {
  return { seq, reason } as InboxItem
}

const idle = { panelOpen: false, resync: false }

describe("gateLiveToasts", () => {
  it("records the newest seq on first load without toasting", () => {
    expect(gateLiveToasts(9, null, idle)).toEqual({ seen: 9, fresh: false })
  })

  it("starts at zero when the inbox is empty", () => {
    expect(gateLiveToasts(0, null, idle)).toEqual({ seen: 0, fresh: false })
  })

  it("is fresh when something newer than the seen seq exists", () => {
    expect(gateLiveToasts(12, 9, idle)).toEqual({ seen: 9, fresh: true })
  })

  it("does nothing when nothing is newer", () => {
    expect(gateLiveToasts(9, 9, idle)).toEqual({ seen: 9, fresh: false })
    expect(gateLiveToasts(4, 9, idle)).toEqual({ seen: 9, fresh: false })
  })

  it("advances past items that arrive while the panel is open", () => {
    const gate = gateLiveToasts(12, 9, { ...idle, panelOpen: true })
    expect(gate).toEqual({ seen: 12, fresh: false })
  })

  it("advances past items when the tab has just come back from hidden", () => {
    const gate = gateLiveToasts(12, 9, { ...idle, resync: true })
    expect(gate).toEqual({ seen: 12, fresh: false })
  })

  it("never lowers the seen seq", () => {
    expect(gateLiveToasts(4, 9, { ...idle, resync: true }).seen).toBe(9)
  })
})

describe("planLiveToasts", () => {
  it("toasts items newer than the seen seq, oldest first", () => {
    const plan = planLiveToasts([item(12), item(11), item(9)], 9)
    expect(plan.items.map((i) => i.seq)).toEqual([11, 12])
    expect(plan.seen).toBe(12)
    expect(plan.overflow).toBe(0)
  })

  it("never toasts workspace activity", () => {
    const plan = planLiveToasts([item(12, null), item(11)], 9)
    expect(plan.items.map((i) => i.seq)).toEqual([11])
    expect(plan.seen).toBe(12)
  })

  it("never lowers the seen seq", () => {
    expect(planLiveToasts([item(4)], 9).seen).toBe(9)
  })

  it("shows three items one by one", () => {
    const plan = planLiveToasts([item(12), item(11), item(10)], 9)
    expect(plan.items).toHaveLength(3)
    expect(plan.overflow).toBe(0)
  })

  it("summarizes more than three as one count", () => {
    const items = [14, 13, 12, 11].map((seq) => item(seq))
    const plan = planLiveToasts(items, 10)
    expect(plan.items).toEqual([])
    expect(plan.overflow).toBe(4)
    expect(plan.seen).toBe(14)
  })

  it("counts only items that would toast toward the summary", () => {
    const items = [item(15, null), item(14), item(13), item(12), item(11)]
    expect(planLiveToasts(items, 10).overflow).toBe(4)
  })
})
