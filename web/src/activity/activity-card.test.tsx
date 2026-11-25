import { act, render, screen } from "@testing-library/react"
import { beforeEach, describe, expect, it, vi } from "vitest"

import { useInboxPanelStore } from "@/inbox/inbox-panel-store"
import type { Dashboard } from "@/spec"

import { ActivityCard } from "./activity-card"

vi.mock("@/dashboards/search", () => ({
  useDashboardSearch: () => ({}),
  useSetDashboardSearch: () => () => {},
}))
vi.mock("@/chat/dashboard-chat", () => ({ DashboardChat: () => null }))
vi.mock("@/share/share-pane", () => ({ SharePane: () => null }))
vi.mock("./edit-pane", () => ({ EditPane: () => null }))
vi.mock("./activity-feed", () => ({
  ActivityFeed: () => <p>Feed of changes</p>,
}))
vi.mock("./activity-store", async (importOriginal) => ({
  ...(await importOriginal<typeof import("./activity-store")>()),
  useCurrentChat: () => undefined,
}))

function renderCard(hidden = false) {
  return render(
    <ActivityCard
      view="activity"
      hidden={hidden}
      dashboardId="d1"
      apiId="d1"
      spec={{ title: "Checkout" } as Dashboard}
      version={1}
      latestVersion={1}
      pinned={false}
      canEdit={false}
      folder={{ id: "f1", title: "Shop", general: false, personal: false }}
      canManage={false}
      madeBy={null}
      back={undefined}
      timeRange={{}}
      selections={null}
      composerRef={{ current: null }}
      guardRef={null}
      onSelectPanel={() => {}}
      onClose={() => {}}
    />
  )
}

beforeEach(() => {
  useInboxPanelStore.setState({ open: false })
})

describe("ActivityCard and the Inbox panel", () => {
  it("hides while the Inbox is open and comes back in the same state when it closes", () => {
    renderCard()
    const card = screen.getByRole("complementary")
    expect(card).toBeVisible()

    act(() => useInboxPanelStore.getState().show())
    expect(card).not.toBeVisible()
    expect(card).toBeInTheDocument()

    act(() => useInboxPanelStore.getState().close())
    expect(card).toBeVisible()
    expect(screen.getByText("Feed of changes")).toBeVisible()
  })

  it("stays hidden on close when the page itself hid it", () => {
    renderCard(true)
    act(() => useInboxPanelStore.getState().show())
    act(() => useInboxPanelStore.getState().close())
    expect(
      screen.getByRole("complementary", { hidden: true })
    ).not.toBeVisible()
  })
})
