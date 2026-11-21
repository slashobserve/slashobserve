import { lazy, Suspense, useId, useMemo, useState } from "react"
import { Link, useNavigate } from "@tanstack/react-router"
import { ChevronDownIcon, ChevronUpIcon, EllipsisIcon } from "lucide-react"
import { toast } from "sonner"

import { Button, buttonVariants } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSub,
  DropdownMenuSubContent,
  DropdownMenuSubTrigger,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { plainTitle } from "@/comments/plain-title"
import { useThreads } from "@/comments/thread-queries"
import { EvidencePanel } from "@/findings/evidence-panel"
import { evidencePanels, type Finding } from "@/findings/queries"
import { observationLink } from "@/lib/links"
import { cn } from "@/lib/utils"
import { sentence, useSaveAnswer, useShareAnswer } from "@/observe/queries"

const FindingMarkdown = lazy(() =>
  import("@/findings/finding-markdown").then((module) => ({
    default: module.FindingMarkdown,
  }))
)

const evidenceHeight = 220
const threadLabelLength = 60

type AnswerMenu = { shareTarget: string | null }

type Props = {
  finding: Finding
  requestId: string
  canSave: boolean
  revise?: { onRevise: (change: string) => void; pending: boolean }
  menu?: AnswerMenu
}

export function evidenceChipText(count: number): string {
  return count === 1 ? "1 evidence panel" : `${count} evidence panels`
}

export function threadLabel(body: string): string {
  const text = plainTitle(body).replace(/\s+/g, " ").trim()
  const chars = [...text]
  return chars.length > threadLabelLength
    ? `${chars.slice(0, threadLabelLength - 1).join("")}…`
    : text
}

function toastError(error: Error) {
  toast.error(sentence(error.message))
}

function CreateObservation({
  requestId,
  variant,
}: {
  requestId: string
  variant: "outline" | "link"
}) {
  const navigate = useNavigate()
  const save = useSaveAnswer(requestId)
  const busy = save.isPending || save.isSuccess
  return (
    <Button
      type="button"
      variant={variant === "link" ? "ghost" : "outline"}
      size="sm"
      className={cn(variant === "link" && "-ml-2 self-start")}
      disabled={busy}
      aria-busy={busy}
      onClick={() =>
        save.mutate(undefined, {
          onSuccess: (saved) =>
            void navigate(observationLink(saved.observation_id)),
          onError: toastError,
        })
      }
    >
      {busy ? "Creating the observation" : "Create observation"}
    </Button>
  )
}

function ThreadChoices({
  target,
  onPick,
}: {
  target: string
  onPick: (threadId: string) => void
}) {
  const { threads, isPending } = useThreads(target)
  const open = threads.filter((entry) => entry.thread.status === "open")
  if (isPending) {
    return <DropdownMenuItem disabled>Loading the threads</DropdownMenuItem>
  }
  if (open.length === 0) {
    return <DropdownMenuItem disabled>No open threads here</DropdownMenuItem>
  }
  return open.map((entry) => (
    <DropdownMenuItem
      key={entry.thread.id}
      onClick={() => onPick(entry.thread.id)}
    >
      <span className="min-w-0 truncate">
        {threadLabel(entry.messages[0]?.body ?? "")}
      </span>
    </DropdownMenuItem>
  ))
}

function Choice({ title, detail }: { title: string; detail: string }) {
  return (
    <span className="flex min-w-0 flex-col">
      <span>{title}</span>
      <span className="text-xs text-muted-foreground">{detail}</span>
    </span>
  )
}

function AnswerMenuButton({
  finding,
  requestId,
  shareTarget,
}: {
  finding: Finding
  requestId: string
  shareTarget: string | null
}) {
  const share = useShareAnswer(requestId, shareTarget ?? "")

  function shareIn(threadId?: string) {
    share.mutate(threadId, {
      onSuccess: () =>
        toast.success(
          threadId === undefined
            ? "Everyone here can see this answer now"
            : "Shared to the thread"
        ),
      onError: toastError,
    })
  }

  async function copy() {
    try {
      if (!navigator.clipboard?.writeText) throw new Error("unavailable")
      await navigator.clipboard.writeText(finding.markdown)
      toast.success("Copied the answer")
    } catch {
      toast.error("Could not copy the answer")
    }
  }

  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={
          <Button
            type="button"
            variant="ghost"
            size="icon-sm"
            aria-label="Answer actions"
            className="ml-auto text-muted-foreground hover:text-foreground"
          />
        }
      >
        <EllipsisIcon aria-hidden="true" />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-[264px]">
        {shareTarget === null ? null : (
          <>
            <DropdownMenuItem
              disabled={share.isPending}
              onClick={() => shareIn()}
            >
              <Choice
                title="Show to everyone"
                detail="Opens a thread with your question and this answer for everyone who can see it."
              />
            </DropdownMenuItem>
            <DropdownMenuSub>
              <DropdownMenuSubTrigger disabled={share.isPending}>
                <Choice
                  title="Share to thread"
                  detail="Adds your question and this answer to an open thread."
                />
              </DropdownMenuSubTrigger>
              <DropdownMenuSubContent className="w-[264px]">
                <ThreadChoices target={shareTarget} onPick={shareIn} />
              </DropdownMenuSubContent>
            </DropdownMenuSub>
          </>
        )}
        <DropdownMenuItem onClick={() => void copy()}>
          <Choice title="Copy answer" detail="Copies the answer as Markdown." />
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

export function AnswerCard({
  finding,
  requestId,
  canSave,
  revise,
  menu,
}: Props) {
  const panels = useMemo(() => evidencePanels(finding), [finding])
  const [unfolded, setUnfolded] = useState(false)
  const evidenceId = useId()
  const change = finding.suggested_change
  const observationId = finding.observation_id
  const promoted =
    finding.observe_worthy && canSave && observationId === null && !unfolded
  return (
    <div className="flex flex-col gap-2.5 rounded-card border px-3.5 py-3">
      <div className="text-sm leading-normal">
        <Suspense
          fallback={<p className="whitespace-pre-wrap">{finding.markdown}</p>}
        >
          <FindingMarkdown markdown={finding.markdown} />
        </Suspense>
      </div>
      {change === null || revise === undefined ? null : (
        <p className="text-[13px] wrap-anywhere text-muted-foreground">
          Suggested change: {change}
        </p>
      )}
      <div className="flex flex-wrap items-center gap-2">
        {panels.length === 0 ? null : (
          <Button
            type="button"
            variant="outline"
            size="sm"
            aria-expanded={unfolded}
            aria-controls={evidenceId}
            onClick={() => setUnfolded(!unfolded)}
          >
            {evidenceChipText(panels.length)}
            {unfolded ? (
              <ChevronUpIcon aria-hidden="true" />
            ) : (
              <ChevronDownIcon aria-hidden="true" />
            )}
          </Button>
        )}
        {change === null || revise === undefined ? null : (
          <Button
            type="button"
            size="sm"
            disabled={revise.pending}
            aria-busy={revise.pending}
            onClick={() => revise.onRevise(change)}
          >
            Revise the dashboard
          </Button>
        )}
        {promoted ? (
          <CreateObservation requestId={requestId} variant="outline" />
        ) : null}
        {menu === undefined ? null : (
          <AnswerMenuButton
            finding={finding}
            requestId={requestId}
            shareTarget={menu.shareTarget}
          />
        )}
      </div>
      {unfolded ? (
        <div id={evidenceId} className="flex flex-col gap-2.5">
          <div className="@container">
            <div className="grid grid-cols-1 gap-2.5 @[640px]:grid-cols-2">
              {panels.map((panel) => (
                <EvidencePanel
                  key={panel.id}
                  findingId={finding.id}
                  panel={panel}
                  from={finding.from}
                  to={finding.to}
                  height={evidenceHeight}
                />
              ))}
            </div>
          </div>
          {observationId !== null ? (
            <Link
              {...observationLink(observationId)}
              className={cn(
                buttonVariants({ variant: "ghost", size: "sm" }),
                "-ml-2 self-start"
              )}
            >
              Open the observation
            </Link>
          ) : canSave ? (
            <CreateObservation requestId={requestId} variant="link" />
          ) : null}
        </div>
      ) : null}
    </div>
  )
}
