import { describe, expect, it } from "vitest"

import type { InboxItem } from "./inbox-item"
import { planLiveToasts } from "./live-toasts"

function item(seq: number, reason: InboxItem["reason"] = "mention"): InboxItem {
  return { seq, reason } as InboxItem
}

const idle = { panelOpen: false, resync: false }

describe("planLiveToasts", () => {
  it("records the newest seq on first load without a toast", () => {
    const plan = planLiveToasts([item(9), item(7)], null, idle)
    expect(plan).toEqual({ seen: 9, items: [], overflow: 0 })
  })

  it("starts at zero when the first load is empty", () => {
    expect(planLiveToasts([], null, idle).seen).toBe(0)
  })

  it("toasts items newer than the seen seq, oldest first", () => {
    const plan = planLiveToasts([item(12), item(11), item(9)], 9, idle)
    expect(plan.items.map((i) => i.seq)).toEqual([11, 12])
    expect(plan.seen).toBe(12)
    expect(plan.overflow).toBe(0)
  })

  it("never toasts workspace activity", () => {
    const plan = planLiveToasts([item(12, null), item(11)], 9, idle)
    expect(plan.items.map((i) => i.seq)).toEqual([11])
    expect(plan.seen).toBe(12)
  })

  it("toasts nothing when nothing is newer", () => {
    expect(planLiveToasts([item(9)], 9, idle)).toEqual({
      seen: 9,
      items: [],
      overflow: 0,
    })
  })

  it("never lowers the seen seq", () => {
    expect(planLiveToasts([item(4)], 9, idle).seen).toBe(9)
  })

  it("shows three items one by one", () => {
    const plan = planLiveToasts([item(12), item(11), item(10)], 9, idle)
    expect(plan.items).toHaveLength(3)
    expect(plan.overflow).toBe(0)
  })

  it("summarizes more than three as one count", () => {
    const items = [14, 13, 12, 11].map((seq) => item(seq))
    const plan = planLiveToasts(items, 10, idle)
    expect(plan.items).toEqual([])
    expect(plan.overflow).toBe(4)
    expect(plan.seen).toBe(14)
  })

  it("counts only items that would toast toward the summary", () => {
    const items = [item(15, null), item(14), item(13), item(12), item(11)]
    expect(planLiveToasts(items, 10, idle).overflow).toBe(4)
  })

  it("advances past items that arrive while the panel is open", () => {
    const plan = planLiveToasts([item(12)], 9, { ...idle, panelOpen: true })
    expect(plan).toEqual({ seen: 12, items: [], overflow: 0 })
  })

  it("advances past items when the tab has just come back from hidden", () => {
    const plan = planLiveToasts([item(12)], 9, { ...idle, resync: true })
    expect(plan).toEqual({ seen: 12, items: [], overflow: 0 })
  })
})
