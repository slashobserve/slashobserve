import { useId, useRef, useState, type FormEvent, type RefObject } from "react"
import { useIsFetching, useQuery, useQueryClient } from "@tanstack/react-query"
import { useRouteContext } from "@tanstack/react-router"
import { LoaderCircleIcon, Trash2Icon } from "lucide-react"
import { toast } from "sonner"

import { outboundDomainKeys } from "@/api/keys"
import { Alert, AlertDescription } from "@/components/ui/alert"
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog"
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
  originText,
  outboundDomainsQuery,
  removeConsequence,
  useAllowDomain,
  useRemoveDomain,
  type OutboundDomain,
  type OutboundDomains,
} from "./outbound-domains"
import { SettingsCard, SettingsProblem, SettingsRow } from "./settings-card"
import { SettingsSection } from "./settings-section"

const description =
  "Sites the server may fetch for dashboards, such as news feeds. Nothing else is fetched."
const cardHint =
  "Webhook addresses must be on this list. A Grafana connection adds its own host."

export function OutboundAccessSection() {
  return (
    <SettingsSection title="Outbound access" description={description}>
      <AllowedDomainsCard />
    </SettingsSection>
  )
}

function AllowedDomainsCard() {
  const query = useQuery(outboundDomainsQuery())
  const title = "Allowed domains"
  if (query.isError && query.data === undefined) {
    return (
      <SettingsCard title={title} hint={cardHint}>
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
      <SettingsCard title={title} hint={cardHint}>
        <SettingsRow label="Allowed domains">
          <Skeleton className="h-4 w-32" />
        </SettingsRow>
      </SettingsCard>
    )
  }

  return (
    <SettingsCard title={title} hint={cardHint}>
      <AllowedDomains list={list} />
    </SettingsCard>
  )
}

function AllowedDomains({ list }: { list: OutboundDomains }) {
  const remove = useRemoveDomain()
  const removeButtons = useRef(new Map<string, HTMLButtonElement>())
  const input = useRef<HTMLInputElement>(null)
  const [chosen, setChosen] = useState<string | null>(null)
  const [allowProblem, setAllowProblem] = useState<string | null>(null)
  const queryClient = useQueryClient()
  const refreshing = useIsFetching({ queryKey: outboundDomainKeys.list() }) > 0
  const confirming =
    list.domains.find((entry) => entry.domain === chosen) ?? null
  const { principal } = useRouteContext({ from: "/_authed" })
  const managedByConfig = list.managed_by === "config"
  const isAdmin = roleAtLeast(principal.role, "admin")
  const locked = managedByConfig || !isAdmin

  function removeDomain(domain: string) {
    const names = list.domains.map((entry) => entry.domain)
    const index = names.indexOf(domain)
    const neighbour = names[index + 1] ?? names[index - 1]
    remove.mutate(domain, {
      onSettled: () => setChosen(null),
      onSuccess: () => {
        setAllowProblem(null)
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
        <p className="px-4 py-3 text-[13px] text-muted-foreground">
          No domains are allowed, so the server makes no outbound requests for
          dashboards.
        </p>
      ) : (
        <ul className="divide-y">
          {list.domains.map((entry) => (
            <DomainRow
              key={entry.domain}
              entry={entry}
              locked={locked}
              removing={remove.isPending && remove.variables === entry.domain}
              onRemove={() => {
                setChosen(entry.domain)
                void queryClient.invalidateQueries({
                  queryKey: outboundDomainKeys.list(),
                })
              }}
              buttonRef={(button) => {
                if (button === null) removeButtons.current.delete(entry.domain)
                else removeButtons.current.set(entry.domain, button)
              }}
            />
          ))}
        </ul>
      )}
      <AlertDialog
        open={confirming !== null}
        onOpenChange={(open) => {
          if (!open && !remove.isPending) setChosen(null)
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle className="wrap-anywhere">
              Stop allowing {confirming?.domain}?
            </AlertDialogTitle>
            <AlertDialogDescription>
              {confirming === null ? null : removeConsequence(confirming)} Allow
              the domain again to undo this.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel aria-disabled={remove.isPending}>
              Cancel
            </AlertDialogCancel>
            <AlertDialogAction
              variant="destructive"
              disabled={remove.isPending || refreshing}
              focusableWhenDisabled
              aria-busy={remove.isPending}
              onClick={() => {
                if (confirming !== null) removeDomain(confirming.domain)
              }}
            >
              {remove.isPending ? (
                <LoaderCircleIcon
                  aria-hidden="true"
                  className="motion-safe:animate-spin"
                />
              ) : null}
              Remove domain
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
      {managedByConfig ? (
        <p className="border-t px-4 py-3 text-[13px] text-muted-foreground">
          Set by SLASHOBSERVE_OUTBOUND_DOMAINS on the server. Change it there
          and restart.
        </p>
      ) : isAdmin ? (
        <AllowDomainForm
          input={input}
          problem={allowProblem}
          onProblem={setAllowProblem}
        />
      ) : (
        <RoleNote className="border-t px-4 py-3">
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
    <li className="flex items-center gap-x-4 px-4 py-3">
      <div className="min-w-0 flex-1">
        <div className="font-mono text-[13px] wrap-anywhere">
          {entry.domain}
        </div>
        <div className="text-xs text-muted-foreground">
          {matchText(entry.domain)}
          {entry.added_at === null ? null : (
            <>
              {" - "}
              {originText(entry)} <RelativeTime iso={entry.added_at} />
            </>
          )}
        </div>
      </div>
      {locked ? null : (
        <Button
          ref={buttonRef}
          variant="outline"
          size="icon-sm"
          className="narrow:size-11"
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
  problem,
  onProblem,
}: {
  input: RefObject<HTMLInputElement | null>
  problem: string | null
  onProblem: (problem: string | null) => void
}) {
  const [draft, setDraft] = useState("")
  const draftErrorId = useId()
  const allow = useAllowDomain()
  const draftError = domainDraftError(draft)
  const empty = draft.trim() === ""

  function submit(event: FormEvent) {
    event.preventDefault()
    if (empty || draftError !== null || allow.isPending) return
    onProblem(null)
    allow.mutate(draft.trim(), {
      onSuccess: () => {
        setDraft("")
        input.current?.focus()
      },
      onError: (error) => onProblem(domainProblemText(error)),
    })
  }

  return (
    <form onSubmit={submit} className="flex flex-col gap-2 border-t px-4 py-3">
      <div className="flex flex-wrap items-start gap-2">
        <div className="min-w-[240px] flex-1">
          <Input
            ref={input}
            className="narrow:h-11"
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
              onProblem(null)
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
          className="narrow:h-11"
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
      {problem === null || empty ? null : (
        <Alert variant="destructive">
          <AlertDescription>{problem}</AlertDescription>
        </Alert>
      )}
    </form>
  )
}
