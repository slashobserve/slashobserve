import { lazy, Suspense, useId, useMemo, useState, type ReactNode } from "react"
import { Link, useNavigate, useParams } from "@tanstack/react-router"
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
import { ApiError, problemText, type ProblemTexts } from "@/api/errors"
import { useThreads } from "@/comments/thread-queries"
import { DuplicateAction } from "@/dashboards/duplicate-action"
import { starterNote, useLatestChange } from "@/dashboards/permissions"
import { EvidencePanel } from "@/findings/evidence-panel"
import { MakeDashboardAction, PinAction } from "@/findings/finding-actions"
import { evidencePanels, type Finding } from "@/findings/queries"
import { dashboardLink, observationLink } from "@/lib/links"
import { cn } from "@/lib/utils"
import {
  useSaveAnswer,
  useShareAnswer,
  useWithdrawShare,
} from "@/observe/queries"

import { useRevisionsAsked } from "./revisions-asked"
import { subjectOf } from "./send-plan"

const FindingMarkdown = lazy(() =>
  import("@/findings/finding-markdown").then((module) => ({
    default: module.FindingMarkdown,
  }))
)

const evidenceHeight = 220
const threadLabelLength = 60

type AnswerMenu = {
  shareTarget: string | null
  sharedThreadId: string | null
  showToEveryone?: boolean | undefined
  subjectObservationId: string | null
}

export type RevisionAsked = boolean | string

export const starterCardNote = `${starterNote} Duplicate it to get a copy you can change.`

export const reviseProblems: ProblemTexts = {
  codes: { provisioned: starterNote },
  fallback: "The change could not be asked for. Try again.",
}

type Props = {
  finding: Finding
  requestId: string
  canSave: boolean
  revise?:
    | {
        onRevise: () => Promise<RevisionAsked>
        pending: boolean
        blocked: string | null
        note?: string | undefined
        dashboardId: string
      }
    | undefined
  menu?: AnswerMenu
  chatId?: string | undefined
  actions?: ReactNode
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
  toast.error(problemText(error))
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

function OpenObservation({ observationId }: { observationId: string }) {
  return (
    <Link
      {...observationLink(observationId)}
      className={cn(buttonVariants({ variant: "ghost", size: "sm" }))}
    >
      Open the observation
    </Link>
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

function SharedThreadItem({
  target,
  threadId,
}: {
  target: string
  threadId: string
}) {
  const subject = subjectOf(target)
  const here = useParams({ strict: false }).dashboardId === subject?.id
  const link =
    subject?.kind === "dashboard" ? (
      here ? (
        <Link
          from="/d/$dashboardId/{-$slug}"
          to="."
          search={(prev) => ({
            ...prev,
            tab: undefined,
            chat: undefined,
            thread: threadId,
          })}
        />
      ) : (
        <Link {...dashboardLink(subject.id)} search={{ thread: threadId }} />
      )
    ) : subject?.kind === "observation" ? (
      <Link {...observationLink(subject.id)} />
    ) : null
  if (link === null) return null
  return (
    <DropdownMenuItem render={link}>
      <Choice
        title="Shared in the thread"
        detail="Opens the thread with your question and this answer."
      />
    </DropdownMenuItem>
  )
}

function showDetail(onPage: boolean): string {
  return onPage
    ? "Opens a thread with your question and this answer, and writes the answer onto the page."
    : "Opens a thread with your question and this answer for everyone who can see it."
}

function withdrawDetail(onPage: boolean): string {
  const base =
    "Removes your question and this answer from the thread, and the thread too when sharing opened it, while no one else has written in it."
  return onPage ? `${base} Blocks it already wrote on the page stay.` : base
}

function AnswerMenuButton({
  finding,
  requestId,
  menu,
}: {
  finding: Finding
  requestId: string
  menu: AnswerMenu
}) {
  const { shareTarget } = menu
  const share = useShareAnswer(requestId, shareTarget ?? "")
  const withdraw = useWithdrawShare(requestId, shareTarget ?? "")
  const [sharedHere, setSharedHere] = useState<string | null | undefined>()
  const sharedThreadId =
    sharedHere === undefined ? menu.sharedThreadId : sharedHere
  const busy = share.isPending || withdraw.isPending
  const shareable =
    finding.observation_id === null ||
    (menu.subjectObservationId !== null &&
      finding.observation_id !== menu.subjectObservationId)
  const onPage =
    shareTarget !== null && subjectOf(shareTarget)?.kind === "observation"

  function shareIn(threadId?: string) {
    share.mutate(threadId, {
      onSuccess: (result) => {
        setSharedHere(result.thread_id)
        toast.success(
          result.replayed
            ? "This answer is already shared in its thread"
            : threadId === undefined
              ? "Everyone here can see this answer now"
              : "Shared to the thread"
        )
      },
      onError: toastError,
    })
  }

  function withdrawFrom(threadId: string) {
    withdraw.mutate(threadId, {
      onSuccess: () => {
        setSharedHere(null)
        toast.success("Only you and Admins can see this answer again")
      },
      onError: (error) => {
        if (error instanceof ApiError && error.status === 404) {
          setSharedHere(null)
        }
        toast.error(
          problemText(error, {
            codes: {
              others_wrote:
                "Someone else wrote in the thread, so the answer stays shared.",
            },
            notFound: "This answer is no longer shared in a thread.",
          })
        )
      },
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
        {shareTarget === null ? null : sharedThreadId !== null ? (
          <>
            <SharedThreadItem target={shareTarget} threadId={sharedThreadId} />
            <DropdownMenuItem
              disabled={busy}
              onClick={() => withdrawFrom(sharedThreadId)}
            >
              <Choice title="Withdraw" detail={withdrawDetail(onPage)} />
            </DropdownMenuItem>
          </>
        ) : !shareable ? null : (
          <>
            {menu.showToEveryone === false ? null : (
              <DropdownMenuItem disabled={busy} onClick={() => shareIn()}>
                <Choice title="Show to everyone" detail={showDetail(onPage)} />
              </DropdownMenuItem>
            )}
            <DropdownMenuSub>
              <DropdownMenuSubTrigger disabled={busy}>
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
  chatId,
  actions,
}: Props) {
  const panels = useMemo(() => evidencePanels(finding), [finding])
  const [unfolded, setUnfolded] = useState(false)
  const [refusal, setRefusal] = useState<string | null>(null)
  const revisionAsked = useRevisionsAsked((state) =>
    state.findingIds.has(finding.id)
  )
  const markAsked = useRevisionsAsked((state) => state.markAsked)
  const evidenceId = useId()
  const blockedId = useId()
  const refusalId = useId()
  const noteId = useId()
  const latest = useLatestChange(revise?.dashboardId ?? null)
  const refused = latest?.content === "none"
  const offered = refused ? undefined : revise
  const starter = refused && latest.provisioned ? revise : undefined
  const blocked = offered?.blocked ?? null
  const reviseNote = offered?.note
  const describedBy =
    [
      blocked === null ? null : blockedId,
      refusal === null ? null : refusalId,
      reviseNote === undefined ? null : noteId,
    ]
      .filter((id) => id !== null)
      .join(" ") || undefined
  const change = finding.suggested_change
  const observationId = finding.observation_id
  const createInRow = canSave && observationId === null && !unfolded
  return (
    <div className="flex flex-col gap-2.5 rounded-card border px-3.5 py-3">
      <div className="text-sm leading-normal">
        <Suspense
          fallback={<p className="whitespace-pre-wrap">{finding.markdown}</p>}
        >
          <FindingMarkdown markdown={finding.markdown} />
        </Suspense>
      </div>
      {change === null ? null : (
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
        {change === null || offered === undefined ? null : (
          <Button
            type="button"
            size="sm"
            disabled={offered.pending || revisionAsked || blocked !== null}
            focusableWhenDisabled
            aria-busy={offered.pending}
            aria-describedby={describedBy}
            onClick={() => {
              setRefusal(null)
              void offered.onRevise().then((asked) => {
                if (typeof asked === "string" && asked !== "") {
                  setRefusal(asked)
                } else if (asked) {
                  markAsked(finding.id)
                }
              })
            }}
          >
            {revisionAsked ? "Revision asked for" : "Revise the dashboard"}
          </Button>
        )}
        {createInRow ? (
          <CreateObservation requestId={requestId} variant="outline" />
        ) : null}
        {observationId === null ? null : (
          <>
            <PinAction finding={finding} />
            {panels.length > 0 ? (
              <MakeDashboardAction finding={finding} chatId={chatId} />
            ) : null}
            <OpenObservation observationId={observationId} />
          </>
        )}
        {actions}
        {menu === undefined ? null : (
          <AnswerMenuButton
            finding={finding}
            requestId={requestId}
            menu={menu}
          />
        )}
      </div>
      {offered === undefined || change === null ? null : (
        <span role="status" className="sr-only">
          {revisionAsked ? "The revision was asked for." : ""}
        </span>
      )}
      {blocked === null || offered === undefined || change === null ? null : (
        <p id={blockedId} className="text-xs text-muted-foreground">
          {blocked}
        </p>
      )}
      {refusal === null || offered === undefined || change === null ? null : (
        <p id={refusalId} role="alert" className="text-xs text-destructive">
          {refusal}
        </p>
      )}
      {reviseNote === undefined || change === null ? null : (
        <p id={noteId} className="text-xs text-muted-foreground">
          {reviseNote}
        </p>
      )}
      {starter !== undefined && change !== null ? (
        <div className="flex flex-wrap items-center gap-2">
          <p className="text-xs text-muted-foreground">{starterCardNote}</p>
          <DuplicateAction dashboardId={starter.dashboardId} />
        </div>
      ) : null}
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
          {observationId === null && canSave ? (
            <CreateObservation requestId={requestId} variant="link" />
          ) : null}
        </div>
      ) : null}
    </div>
  )
}
