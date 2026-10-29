import {
  useEffect,
  useId,
  useRef,
  useState,
  type FormEvent,
  type RefObject,
} from "react"
import { useQuery } from "@tanstack/react-query"
import { useLocation, useRouteContext } from "@tanstack/react-router"
import { LoaderCircleIcon, Trash2Icon } from "lucide-react"
import { toast } from "sonner"

import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import { RelativeTime } from "@/components/relative-time"
import { RoleNote } from "@/components/role-note"
import { roleAtLeast } from "@/lib/roles"
import { loadProblemText } from "@/service-accounts/queries"

import {
  domainDraftError,
  domainProblemText,
  matchText,
  outboundDomainsQuery,
  useAllowDomain,
  useRemoveDomain,
  type OutboundDomain,
  type OutboundDomains,
} from "./outbound-domains"
import { SettingsCard, SettingsProblem, SettingsRow } from "./settings-card"

const outboundAccessId = "outbound-access"

const hint =
  "Sites the server may fetch for dashboards, such as news feeds. Nothing else is fetched."

export function OutboundAccessCard() {
  const query = useQuery(outboundDomainsQuery())
  const hash = useLocation({ select: (location) => location.hash })
  const settled = !query.isPending
  useEffect(() => {
    if (hash !== outboundAccessId || !settled) return
    document
      .getElementById(outboundAccessId)
      ?.scrollIntoView({ block: "start" })
  }, [hash, settled])

  if (query.isError && query.data === undefined) {
    return (
      <SettingsCard id={outboundAccessId} title="Outbound access" hint={hint}>
        <SettingsProblem
          title="Could not load outbound access"
          detail={loadProblemText(query.error)}
          retrying={query.isFetching}
          onRetry={() => void query.refetch()}
        />
      </SettingsCard>
    )
  }

  const list = query.data
  if (list === undefined) {
    return (
      <SettingsCard id={outboundAccessId} title="Outbound access" hint={hint}>
        <SettingsRow label="Allowed domains">
          <Skeleton className="h-4 w-32" />
        </SettingsRow>
      </SettingsCard>
    )
  }

  return (
    <SettingsCard id={outboundAccessId} title="Outbound access" hint={hint}>
      <AllowedDomains list={list} />
    </SettingsCard>
  )
}

function AllowedDomains({ list }: { list: OutboundDomains }) {
  const remove = useRemoveDomain()
  const removeButtons = useRef(new Map<string, HTMLButtonElement>())
  const input = useRef<HTMLInputElement>(null)
  const { principal } = useRouteContext({ from: "/_authed" })
  const managedByConfig = list.managed_by === "config"
  const isAdmin = roleAtLeast(principal.role, "admin")
  const locked = managedByConfig || !isAdmin

  function removeDomain(domain: string) {
    const names = list.domains.map((entry) => entry.domain)
    const index = names.indexOf(domain)
    const neighbour = names[index + 1] ?? names[index - 1]
    remove.mutate(domain, {
      onSuccess: () => {
        toast.success(`Removed ${domain}`)
        const next =
          neighbour === undefined
            ? input.current
            : removeButtons.current.get(neighbour)
        next?.focus()
      },
      onError: (error) => toast.error(domainProblemText(error)),
    })
  }

  return (
    <>
      {list.domains.length === 0 ? (
        <p className="border-b px-4 py-3 text-[13px] text-muted-foreground">
          No domains are allowed, so the server makes no outbound requests for
          dashboards.
        </p>
      ) : (
        <ul>
          {list.domains.map((entry) => (
            <DomainRow
              key={entry.domain}
              entry={entry}
              locked={locked}
              removing={remove.isPending && remove.variables === entry.domain}
              onRemove={() => removeDomain(entry.domain)}
              buttonRef={(button) => {
                if (button === null) removeButtons.current.delete(entry.domain)
                else removeButtons.current.set(entry.domain, button)
              }}
            />
          ))}
        </ul>
      )}
      {managedByConfig ? (
        <p className="px-4 py-3 text-[13px] text-muted-foreground">
          Set by SLASHOBSERVE_OUTBOUND_DOMAINS on the server. Change it there
          and restart.
        </p>
      ) : isAdmin ? (
        <AllowDomainForm input={input} />
      ) : (
        <RoleNote className="px-4 py-3">
          Only Admins can change outbound access.
        </RoleNote>
      )}
    </>
  )
}

function DomainRow({
  entry,
  locked,
  removing,
  onRemove,
  buttonRef,
}: {
  entry: OutboundDomain
  locked: boolean
  removing: boolean
  onRemove: () => void
  buttonRef: (button: HTMLButtonElement | null) => void
}) {
  return (
    <li className="flex items-center gap-x-4 border-b px-4 py-3">
      <div className="min-w-0 flex-1">
        <div className="font-mono text-[13px] wrap-anywhere">
          {entry.domain}
        </div>
        <div className="text-xs text-muted-foreground">
          {matchText(entry.domain)}
          {entry.added_at === null ? null : (
            <>
              {" - "}
              {entry.added_by === null
                ? "Added "
                : `Added by ${entry.added_by} `}
              <RelativeTime iso={entry.added_at} />
            </>
          )}
        </div>
      </div>
      {locked ? null : (
        <Button
          ref={buttonRef}
          variant="outline"
          size="icon-sm"
          aria-label={`Remove ${entry.domain}`}
          disabled={removing}
          focusableWhenDisabled
          aria-busy={removing}
          onClick={onRemove}
        >
          {removing ? (
            <LoaderCircleIcon
              aria-hidden="true"
              className="motion-safe:animate-spin"
            />
          ) : (
            <Trash2Icon aria-hidden="true" />
          )}
        </Button>
      )}
    </li>
  )
}

function AllowDomainForm({
  input,
}: {
  input: RefObject<HTMLInputElement | null>
}) {
  const [draft, setDraft] = useState("")
  const [problem, setProblem] = useState<string | null>(null)
  const draftErrorId = useId()
  const allow = useAllowDomain()
  const draftError = domainDraftError(draft)
  const empty = draft.trim() === ""

  function submit(event: FormEvent) {
    event.preventDefault()
    if (empty || draftError !== null || allow.isPending) return
    setProblem(null)
    allow.mutate(draft.trim(), {
      onSuccess: () => {
        setDraft("")
        input.current?.focus()
      },
      onError: (error) => setProblem(domainProblemText(error)),
    })
  }

  return (
    <form onSubmit={submit} className="flex flex-col gap-2 px-4 py-3">
      <div className="flex flex-wrap items-start gap-2">
        <div className="min-w-[240px] flex-1">
          <Input
            ref={input}
            aria-label="Domain"
            placeholder="example.com or *.example.com"
            maxLength={256}
            autoComplete="off"
            spellCheck={false}
            value={draft}
            aria-invalid={draftError !== null}
            aria-describedby={draftError === null ? undefined : draftErrorId}
            onChange={(event) => {
              setDraft(event.target.value)
              setProblem(null)
            }}
          />
          {draftError === null ? null : (
            <p id={draftErrorId} className="mt-1 text-xs text-destructive">
              {draftError}
            </p>
          )}
        </div>
        <Button
          type="submit"
          disabled={empty || draftError !== null || allow.isPending}
          aria-busy={allow.isPending}
        >
          {allow.isPending ? (
            <LoaderCircleIcon
              aria-hidden="true"
              className="motion-safe:animate-spin"
            />
          ) : null}
          Allow domain
        </Button>
      </div>
      {problem === null ? null : (
        <Alert variant="destructive">
          <AlertDescription>{problem}</AlertDescription>
        </Alert>
      )}
    </form>
  )
}
