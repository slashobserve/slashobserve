import { useEffect, useState, type FormEvent, type RefObject } from "react"
import { useQueryClient, type QueryClient } from "@tanstack/react-query"
import { useNavigate } from "@tanstack/react-router"
import { toast } from "sonner"

import { loadProblemText } from "@/api/errors"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Switch } from "@/components/ui/switch"
import { Textarea } from "@/components/ui/textarea"
import { asDashboardSpec, latestDashboardQuery } from "@/dashboards/queries"
import { datasourcesQuery } from "@/datasources/queries"
import { observationLink } from "@/lib/links"
import { cleanText, lineRule, textError } from "@/lib/user-text"
import { useAgentInfo, useCreateObserveRequest } from "@/observe/queries"

import { FolderSelect } from "./folder-select"
import {
  dashboardPanelBody,
  linkLabel,
  logsLinkBody,
  noteLinks,
  traceLinkBody,
  type LinkOutcome,
  type NoteLink,
} from "./note-links"
import { useAddBlockTo, type useCreateObservation } from "./queries"

type Props = {
  create: ReturnType<typeof useCreateObservation>
  formId: string
  titleRef: RefObject<HTMLInputElement | null>
  autoFocus?: boolean
  withNote?: boolean
  withAgentStart?: boolean
  onBusyChange?: (busy: boolean) => void
}

const maxTitleLength = 200
const maxNoteLength = 4000
const agentStartHint =
  "It reads the note, runs the first queries and writes what it finds as blocks you can keep or discard."
const noteHint =
  "Links to a logs search, a trace or a dashboard panel in this app become blocks."

function titleError(title: string): string | null {
  return textError("title", title, lineRule(maxTitleLength))
}

const reasonsShown = 3

function unaddedLinksText(count: number): string {
  return count === 1
    ? "1 link in the note could not be added as a block."
    : `${count} links in the note could not be added as blocks.`
}

function unaddedLinksReasons(reasons: readonly string[]): string {
  const shown = reasons.slice(0, reasonsShown).join(" ")
  const more = reasons.length - reasonsShown
  return more > 0 ? `${shown} And ${more} more.` : shown
}

async function datasourceType(
  queryClient: QueryClient,
  uid: string
): Promise<string | undefined> {
  const listed = await queryClient.ensureQueryData(datasourcesQuery())
  return listed.datasources.find((one) => one.uid === uid)?.type
}

async function linkBody(
  queryClient: QueryClient,
  link: NoteLink
): Promise<LinkOutcome> {
  const now = Date.now()
  switch (link.kind) {
    case "logs": {
      if (link.search.f === undefined) {
        return { body: logsLinkBody(link, undefined, now) }
      }
      const type = await datasourceType(queryClient, link.uid)
      return { body: logsLinkBody(link, type, now) }
    }
    case "trace": {
      const type = await datasourceType(queryClient, link.uid)
      return { body: traceLinkBody(link, type, now) }
    }
    case "panel": {
      const envelope = await queryClient.fetchQuery(
        latestDashboardQuery(queryClient, link.dashboardId)
      )
      return dashboardPanelBody(
        link,
        asDashboardSpec(envelope.version.spec),
        now
      )
    }
  }
}

function useStartFollowUps() {
  const queryClient = useQueryClient()
  const addBlock = useAddBlockTo()
  const observe = useCreateObserveRequest()

  return async function followUps(input: {
    observationId: string
    links: NoteLink[]
    prompt: string | null
  }) {
    const reasons: string[] = []
    for (const link of input.links) {
      try {
        const outcome = await linkBody(queryClient, link)
        if ("reason" in outcome) {
          reasons.push(`${linkLabel(link)}: ${outcome.reason}`)
          continue
        }
        await addBlock.mutateAsync({
          observationId: input.observationId,
          kind: "panel",
          body: outcome.body,
        })
      } catch (error) {
        reasons.push(`${linkLabel(link)}: ${loadProblemText(error)}`)
      }
    }
    if (reasons.length > 0) {
      toast.error(unaddedLinksText(reasons.length), {
        description: unaddedLinksReasons(reasons),
      })
    }
    if (input.prompt === null) return
    try {
      await observe.mutateAsync({
        prompt: input.prompt,
        kind: "investigate",
        observationId: input.observationId,
      })
    } catch (error) {
      toast.error(
        `The observation was created, but Observe Agent did not start. ${loadProblemText(error)}`
      )
    }
  }
}

function AgentStartField({
  id,
  checked,
  disabled,
  onChange,
}: {
  id: string
  checked: boolean
  disabled: boolean
  onChange: (checked: boolean) => void
}) {
  const agent = useAgentInfo()
  if (agent.data?.available !== true) return null
  return (
    <div className="flex flex-col gap-1.5">
      <div className="flex items-center justify-between gap-3">
        <Label htmlFor={id}>Ask Observe Agent to start</Label>
        <Switch
          id={id}
          checked={checked}
          disabled={disabled}
          aria-describedby={`${id}-hint`}
          onCheckedChange={onChange}
        />
      </div>
      <p id={`${id}-hint`} className="text-xs text-muted-foreground">
        {agentStartHint}
      </p>
    </div>
  )
}

export function NewObservationForm({
  create,
  formId,
  titleRef,
  autoFocus = false,
  withNote = false,
  withAgentStart = false,
  onBusyChange,
}: Props) {
  const navigate = useNavigate()
  const followUps = useStartFollowUps()
  const [title, setTitle] = useState("")
  const [note, setNote] = useState("")
  const [folderId, setFolderId] = useState<string | null>(null)
  const [agentStart, setAgentStart] = useState(false)
  const [submitted, setSubmitted] = useState(false)
  const [starting, setStarting] = useState(false)
  const [problem, setProblem] = useState<string | null>(null)

  useEffect(() => {
    if (autoFocus) titleRef.current?.focus()
  }, [autoFocus, titleRef])

  const busy = starting || create.isPending
  useEffect(() => {
    onBusyChange?.(busy)
  }, [busy, onBusyChange])

  const invalid = titleError(title)
  const showError = invalid !== null && (submitted || title.length > 0)
  const errorId = `${formId}-title-error`

  async function submit(event: FormEvent) {
    event.preventDefault()
    setSubmitted(true)
    if (busy) return
    if (invalid !== null) {
      titleRef.current?.focus()
      return
    }
    setProblem(null)
    setStarting(true)
    const text = withNote ? note.trim() : ""
    try {
      const envelope = await create.mutateAsync({
        title: cleanText(title),
        ...(folderId === null ? {} : { folder_id: folderId }),
        ...(text === ""
          ? {}
          : { first_block: { kind: "text", body: { markdown: text } } }),
      })
      const observationId = envelope.observation.id
      await followUps({
        observationId,
        links: noteLinks(text, window.location.origin),
        prompt: agentStart ? text || cleanText(title) : null,
      })
      void navigate(observationLink(observationId))
    } catch (error) {
      setProblem(loadProblemText(error))
    } finally {
      setStarting(false)
    }
  }

  return (
    <form
      id={formId}
      noValidate
      onSubmit={(event) => void submit(event)}
      className="flex flex-col gap-4"
    >
      <div className="flex flex-col gap-1.5">
        <Label htmlFor={`${formId}-title`}>Title</Label>
        <Input
          id={`${formId}-title`}
          ref={titleRef}
          value={title}
          autoComplete="off"
          required
          readOnly={busy}
          aria-invalid={showError}
          aria-describedby={showError ? errorId : undefined}
          onChange={(event) => setTitle(event.target.value)}
        />
        {showError ? (
          <p id={errorId} role="alert" className="text-sm text-destructive">
            {invalid}
          </p>
        ) : null}
      </div>
      {withNote ? (
        <div className="flex flex-col gap-1.5">
          <Label htmlFor={`${formId}-note`}>First note (optional)</Label>
          <Textarea
            id={`${formId}-note`}
            value={note}
            maxLength={maxNoteLength}
            readOnly={busy}
            aria-describedby={`${formId}-note-hint`}
            onChange={(event) => setNote(event.target.value)}
          />
          <p
            id={`${formId}-note-hint`}
            className="text-xs text-muted-foreground"
          >
            {noteHint}
          </p>
        </div>
      ) : null}
      <div className="flex flex-col gap-1.5">
        <Label htmlFor={`${formId}-folder`}>Folder</Label>
        <FolderSelect
          id={`${formId}-folder`}
          value={folderId}
          disabled={busy}
          onChange={setFolderId}
        />
      </div>
      {withAgentStart ? (
        <AgentStartField
          id={`${formId}-agent`}
          checked={agentStart}
          disabled={busy}
          onChange={setAgentStart}
        />
      ) : null}
      {problem !== null ? (
        <Alert variant="destructive">
          <AlertDescription>{problem}</AlertDescription>
        </Alert>
      ) : null}
    </form>
  )
}
