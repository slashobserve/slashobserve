import { useQuery, useQueryClient } from "@tanstack/react-query"
import { useNavigate } from "@tanstack/react-router"

import { Button } from "@/components/ui/button"
import { latestDashboardQuery } from "@/dashboards/queries"
import { DashboardsSection } from "@/home/dashboards-section"
import { askLink } from "@/lib/links"
import { SavedPrompts } from "@/prompts/saved-prompts"
import { dashboardTitlesQuery } from "@/shell/palette-queries"

import { ChatComposer, useOpenSent } from "./chat-composer"
import { composerPlaceholder } from "./prompt-length"
import { RecentChats } from "./recent-chats"
import { useChooseExample } from "./use-choose-example"
import { useExamples } from "./use-examples"

type Props = { initialPrompt?: string; dashboardId?: string }

export function ChatHome({ initialPrompt, dashboardId }: Props) {
  const { examples, agentReady, agentOff } = useExamples()
  const dashboards = useQuery(dashboardTitlesQuery())
  const queryClient = useQueryClient()
  const { choose, pending } = useChooseExample()
  const navigate = useNavigate()
  const openSent = useOpenSent()

  const listed = dashboards.data?.find(
    (dashboard) => dashboard.id === dashboardId
  )
  const lookup = useQuery({
    ...latestDashboardQuery(queryClient, dashboardId ?? ""),
    enabled:
      dashboardId !== undefined &&
      !dashboards.isPending &&
      listed === undefined,
    retry: false,
  })
  const found =
    listed ??
    (lookup.data === undefined
      ? undefined
      : {
          id: lookup.data.dashboard.id,
          title: lookup.data.dashboard.title,
          canEdit: lookup.data.dashboard.permissions.edit,
        })
  const revising = found?.canEdit === true ? found : undefined
  const resolving =
    dashboardId !== undefined &&
    found === undefined &&
    (dashboards.isPending || lookup.isLoading)

  return (
    <div className="flex min-h-full flex-col items-center justify-center gap-3.5 pt-10 pb-20">
      <h1 className="mb-1.5 text-center text-[26px] font-semibold tracking-[-0.01em]">
        What do you want to see?
      </h1>
      {agentOff ? <DashboardsSection /> : null}
      <div className="w-full max-w-[720px] min-[1800px]:max-w-[960px]">
        <ChatComposer
          key={initialPrompt ?? ""}
          rows={3}
          disabled={resolving}
          dashboardId={revising?.id}
          dashboardTitle={revising?.title}
          onRemoveDashboard={
            revising === undefined
              ? undefined
              : () =>
                  void navigate({
                    ...askLink(initialPrompt ?? ""),
                    replace: true,
                  })
          }
          placeholder={composerPlaceholder}
          initialPrompt={initialPrompt}
          autoFocus
          promptPicker
          onSent={openSent}
        />
      </div>
      {!agentReady || examples.length === 0 ? null : (
        <div
          role="group"
          aria-label="Example prompts"
          className="flex max-w-[720px] flex-wrap justify-center gap-2 min-[1800px]:max-w-[960px]"
        >
          {examples.map((example) => (
            <Button
              key={example.label}
              type="button"
              variant="outline"
              className="font-normal text-muted-foreground"
              disabled={pending}
              onClick={() => choose(example)}
            >
              {example.label}
            </Button>
          ))}
        </div>
      )}
      {agentReady ? <DashboardsSection /> : null}
      <SavedPrompts />
      <RecentChats />
    </div>
  )
}
