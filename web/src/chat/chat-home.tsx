import { useQuery, useQueryClient } from "@tanstack/react-query"
import { useNavigate } from "@tanstack/react-router"

import { Button } from "@/components/ui/button"
import { Composer } from "@/conversation/composer"
import { ungated } from "@/conversation/delivery"
import { useComposer } from "@/conversation/use-composer"
import { latestDashboardQuery } from "@/dashboards/queries"
import { DashboardsSection } from "@/home/dashboards-section"
import { askLink } from "@/lib/links"
import { SavedPrompts } from "@/prompts/saved-prompts"
import { dashboardTitlesQuery } from "@/shell/palette-queries"

import { useOpenSentTurn } from "./open-sent"
import { composerPlaceholder } from "./prompt-length"
import { RecentChats } from "./recent-chats"
import { useChooseExample } from "./use-choose-example"
import { useExamples } from "./use-examples"

type Props = {
  initialPrompt?: string | undefined
  dashboardId?: string | undefined
}

function HomeComposer({
  initialPrompt,
  about,
  disabled,
  onRemoveDashboard,
}: {
  initialPrompt?: string | undefined
  about: { id: string; title: string } | undefined
  disabled: boolean
  onRemoveDashboard: () => void
}) {
  const model = useComposer({
    subject: about === undefined ? null : { kind: "dashboard", id: about.id },
    place: { kind: "chat", chatId: null },
    gate: ungated,
    initialText: initialPrompt,
    onSent: useOpenSentTurn(),
  })
  return (
    <Composer
      model={model}
      layout="page"
      rows={3}
      disabled={disabled}
      contextChips={
        about === undefined
          ? []
          : [
              {
                id: "dashboard",
                label: about.title,
                onRemove: onRemoveDashboard,
              },
            ]
      }
      placeholder={composerPlaceholder}
      autoFocus
      promptPicker
    />
  )
}

export function ChatHome({ initialPrompt, dashboardId }: Props) {
  const { examples, agentReady, agentOff } = useExamples()
  const dashboards = useQuery(dashboardTitlesQuery())
  const queryClient = useQueryClient()
  const { choose, pending } = useChooseExample()
  const navigate = useNavigate()

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
          canComment: lookup.data.dashboard.permissions.comment,
        })
  const about = found?.canComment === true ? found : undefined
  const unaskable = found?.canComment === false ? found : undefined
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
        <HomeComposer
          key={initialPrompt ?? ""}
          initialPrompt={initialPrompt}
          about={about}
          disabled={resolving}
          onRemoveDashboard={() =>
            void navigate({ ...askLink(initialPrompt ?? ""), replace: true })
          }
        />
        {unaskable === undefined ? null : (
          <p className="mt-1.5 text-center text-xs text-muted-foreground">
            You cannot ask about {unaskable.title}, because you cannot comment
            on it; this message asks without it.
          </p>
        )}
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
