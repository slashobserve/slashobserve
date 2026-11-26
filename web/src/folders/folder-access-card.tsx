import { useId, useRef, useState, type FormEvent, type Ref } from "react"
import { useQuery } from "@tanstack/react-query"
import { useRouteContext } from "@tanstack/react-router"
import { LoaderCircleIcon, PlusIcon } from "lucide-react"
import { toast } from "sonner"

import { instanceQuery } from "@/api/instance"
import { ApiError } from "@/api/errors"
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
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Skeleton } from "@/components/ui/skeleton"
import { usersQuery } from "@/people/queries"
import { roleAtLeast, roleLabels } from "@/lib/roles"
import { cn } from "@/lib/utils"
import { loadProblemText } from "@/service-accounts/queries"
import {
  SettingsCard,
  SettingsProblem,
  SettingsRow,
} from "@/settings/settings-card"

import { GranteeInput } from "./grantee-input"
import {
  entriesForPut,
  entryKey,
  levelLabels,
  levelOptions,
  parseGrantee,
  type Entry,
  type GrantChange,
  type Level,
} from "./grants"
import {
  folderPermissionsQuery,
  useSetFolderPermissions,
  type FolderPermissions,
} from "./queries"

const title = "Who can see this folder"

const intro =
  "With nobody listed, everyone follows their role. Once you add someone, only the people and groups below, plus admins, can see it."

type Access = "everyone" | "listed" | "admins"

type Confirming = "clear" | "open" | "admins"

const confirmText: Record<Confirming, { description: string; action: string }> =
  {
    clear: {
      description: "Everyone will see this folder by role.",
      action: "Remove everyone",
    },
    open: {
      description: "Everyone will see this folder by role.",
      action: "Open to everyone",
    },
    admins: {
      description:
        "Only Admins will see this folder and everything in it, the person who created it included.",
      action: "Make Admins only",
    },
  }

function confirmTitle(confirming: Confirming, count: number): string {
  if (confirming === "open") return "Open this folder to everyone?"
  if (confirming === "admins") return "Make this folder Admins only?"
  return count === 1
    ? "Remove the 1 person or group?"
    : `Remove all ${count} people and groups?`
}

function peopleText(count: number): string {
  return `${count} ${count === 1 ? "person" : "people"}`
}

function problemText(error: unknown, change: GrantChange | null): string {
  if (error instanceof ApiError && error.code === "unknown_user") {
    const email =
      change?.kind === "add" && "email" in change.grantee
        ? change.grantee.email
        : "This person"
    return `${email} has not signed in yet. They need to sign in once before you can add them.`
  }
  return loadProblemText(error)
}

export function FolderAccessCard({
  folderId,
  readOnly,
}: {
  folderId: string
  readOnly: boolean
}) {
  const query = useQuery(folderPermissionsQuery(folderId))

  if (query.isError && query.data === undefined) {
    return (
      <SettingsCard title={title} hint={intro}>
        <SettingsProblem
          title="Could not load who can see this folder"
          detail={loadProblemText(query.error)}
          retrying={query.isFetching}
          onRetry={() => void query.refetch()}
        />
      </SettingsCard>
    )
  }
  if (query.data === undefined) {
    return (
      <SettingsCard title={title} hint={intro}>
        <SettingsRow label="People and groups">
          <Skeleton className="h-4 w-32" />
        </SettingsRow>
      </SettingsCard>
    )
  }
  return (
    <AccessCard
      folderId={folderId}
      permissions={query.data}
      readOnly={readOnly}
    />
  )
}

function AccessCard({
  folderId,
  permissions,
  readOnly,
}: {
  folderId: string
  permissions: FolderPermissions
  readOnly: boolean
}) {
  const save = useSetFolderPermissions(folderId)
  const { principal } = useRouteContext({ from: "/_authed" })
  const mayChooseAdminsOnly = roleAtLeast(principal.role, "admin")
  const addInput = useRef<HTMLInputElement>(null)
  const [revealed, setRevealed] = useState(false)
  const [confirming, setConfirming] = useState<Confirming | null>(null)
  const [problem, setProblem] = useState<string | null>(null)
  const entries = permissions.entries
  const adminsOnly = permissions.admins_only && !revealed
  const listed = !adminsOnly && (entries.length > 0 || revealed)
  const access: Access = adminsOnly ? "admins" : listed ? "listed" : "everyone"
  const pending = save.isPending
  const listedEmails = new Set(
    entries.flatMap((entry) =>
      entry.user === undefined ? [] : [entry.user.email.toLowerCase()]
    )
  )

  function apply(change: GrantChange, onDone?: () => void) {
    setProblem(null)
    save.mutate(
      { entries: entriesForPut(entries, change) },
      {
        onSuccess: () => {
          toast.success("Access updated")
          onDone?.()
        },
        onError: (error) => setProblem(problemText(error, change)),
      }
    )
  }

  function chooseAccess(next: Access) {
    if (next === "listed") {
      setRevealed(true)
      return
    }
    if (next === "admins") setConfirming("admins")
    else if (entries.length > 0) setConfirming("clear")
    else if (permissions.admins_only) setConfirming("open")
    else setRevealed(false)
  }

  function confirm(target: Confirming) {
    setProblem(null)
    save.mutate(
      { entries: [], admins_only: target === "admins" },
      {
        onSuccess: () => {
          toast.success("Access updated")
          setRevealed(false)
          setConfirming(null)
        },
        onError: (error) => setProblem(problemText(error, null)),
      }
    )
  }

  function changeConfirmOpen(open: boolean) {
    if (open || pending) return
    setConfirming(null)
    setProblem(null)
    save.reset()
  }

  return (
    <SettingsCard title={title} hint={intro}>
      {readOnly ? (
        <p className="border-b px-4 py-3 text-[13px]">
          {adminsOnly
            ? "Admins only"
            : listed
              ? "Only the people and groups below"
              : "Everyone, by role"}
        </p>
      ) : (
        <fieldset className="border-b px-4 py-3">
          <legend className="sr-only">Folder access</legend>
          <RadioGroup
            value={access}
            disabled={pending}
            onValueChange={(next) => chooseAccess(next as Access)}
          >
            <label className="flex items-start gap-2 text-[13px]">
              <RadioGroupItem value="everyone" className="mt-0.5" />
              <span>
                Everyone, by role
                <span className="block text-xs text-muted-foreground">
                  Viewers view, Commenters comment, Editors edit, Admins manage.
                </span>
              </span>
            </label>
            <label className="flex items-start gap-2 text-[13px]">
              <RadioGroupItem value="listed" className="mt-0.5" />
              <span>
                Only the people and groups below
                <span className="block text-xs text-muted-foreground">
                  Hidden from everyone else, including in search, tags and the
                  agent.
                </span>
              </span>
            </label>
            {mayChooseAdminsOnly ? (
              <label className="flex items-start gap-2 text-[13px]">
                <RadioGroupItem value="admins" className="mt-0.5" />
                <span>
                  Admins only
                  <span className="block text-xs text-muted-foreground">
                    Hidden from everyone but Admins, the person who created it
                    included. Content that reads an Admins-only datasource can
                    only be saved in such a folder.
                  </span>
                </span>
              </label>
            ) : null}
          </RadioGroup>
        </fieldset>
      )}
      {listed ? (
        <GrantTable
          entries={entries}
          readOnly={readOnly}
          pending={pending}
          onSetLevel={(entry, level) =>
            apply({ kind: "setLevel", entry, level })
          }
          onRemove={(entry) =>
            apply({ kind: "remove", entry }, () => addInput.current?.focus())
          }
        />
      ) : null}
      {listed && entries.length === 0 ? (
        <p role="status" className="border-b px-4 py-3 text-[13px]">
          Nobody is listed yet, so everyone still sees this folder by role.
          {readOnly ? "" : " Add a person or group to restrict it."}
        </p>
      ) : null}
      {listed && !readOnly ? (
        <AddGrantForm
          inputRef={addInput}
          listedEmails={listedEmails}
          pending={pending}
          onAdd={apply}
        />
      ) : null}
      {problem !== null && confirming === null ? (
        <div className="px-4 pb-3">
          <Alert variant="destructive">
            <AlertDescription>{problem}</AlertDescription>
          </Alert>
        </div>
      ) : null}
      {readOnly ? null : (
        <p className="px-4 pb-3.5 text-xs text-muted-foreground">
          Groups come from the <span className="font-mono">groups</span> claim
          of your identity provider. A person gets the higher of their
          role&apos;s level and their highest entry, so an entry can give a
          Viewer or Commenter more but never gives anyone less. Admins manage
          every folder.
        </p>
      )}
      <AlertDialog open={confirming !== null} onOpenChange={changeConfirmOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              {confirmTitle(confirming ?? "clear", entries.length)}
            </AlertDialogTitle>
            <AlertDialogDescription>
              {confirmText[confirming ?? "clear"].description}
            </AlertDialogDescription>
          </AlertDialogHeader>
          {problem !== null ? (
            <Alert variant="destructive">
              <AlertDescription>{problem}</AlertDescription>
            </Alert>
          ) : null}
          <AlertDialogFooter>
            <AlertDialogCancel aria-disabled={pending}>
              Cancel
            </AlertDialogCancel>
            <AlertDialogAction
              variant="destructive"
              disabled={pending}
              focusableWhenDisabled
              aria-busy={pending}
              onClick={() => confirm(confirming ?? "clear")}
            >
              {pending ? (
                <LoaderCircleIcon
                  aria-hidden="true"
                  className="motion-safe:animate-spin"
                />
              ) : null}
              {confirmText[confirming ?? "clear"].action}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </SettingsCard>
  )
}

function GrantTable({
  entries,
  readOnly,
  pending,
  onSetLevel,
  onRemove,
}: {
  entries: Entry[]
  readOnly: boolean
  pending: boolean
  onSetLevel: (entry: Entry, level: Level) => void
  onRemove: (entry: Entry) => void
}) {
  return (
    <ul aria-label="People and groups with access" className="border-b">
      {entries.map((entry) => {
        const name = entryName(entry)
        return (
          <li
            key={entryKey(entry)}
            className="flex flex-wrap items-center gap-x-3 gap-y-2 border-b px-4 py-3"
          >
            <div className="min-w-0 flex-1 basis-48">
              <GranteeLabel entry={entry} />
            </div>
            <div className="flex items-center gap-2">
              {readOnly ? (
                <span className="text-[13px]">{levelLabels[entry.level]}</span>
              ) : (
                <>
                  <LevelSelect
                    label={`Access for ${name}`}
                    value={entry.level}
                    options={levelOptions(entry)}
                    disabled={pending}
                    onChange={(level) => onSetLevel(entry, level)}
                  />
                  <Button
                    variant="outline"
                    size="sm"
                    className="font-normal"
                    aria-label={`Remove ${name}`}
                    disabled={pending}
                    focusableWhenDisabled
                    onClick={() => onRemove(entry)}
                  >
                    Remove
                  </Button>
                </>
              )}
            </div>
          </li>
        )
      })}
      <li className="flex flex-wrap items-center gap-x-3 gap-y-1 px-4 py-3 text-[13px] text-muted-foreground">
        <span className="flex min-w-0 flex-1 basis-48 items-center gap-2.5">
          <Mark text="A" round={false} />
          Instance admins
        </span>
        <span>Admin, always</span>
      </li>
    </ul>
  )
}

function entryName(entry: Entry): string {
  return entry.kind === "user" ? (entry.user?.name ?? "") : (entry.group ?? "")
}

function Mark({ text, round }: { text: string; round: boolean }) {
  return (
    <span
      aria-hidden="true"
      className={cn(
        "grid size-[26px] flex-none place-items-center border text-[11px] font-semibold",
        round ? "rounded-full" : "rounded-control"
      )}
    >
      {text.slice(0, 1).toUpperCase()}
    </span>
  )
}

function GranteeLabel({ entry }: { entry: Entry }) {
  const name = entryName(entry)
  const detail =
    entry.kind === "user"
      ? entry.user === undefined
        ? ""
        : `${entry.user.email}, ${roleLabels[entry.user.role]} on this server`
      : `Group, ${peopleText(entry.member_count ?? 0)}`
  return (
    <span className="flex items-center gap-2.5">
      <Mark text={name} round={entry.kind === "user"} />
      <span className="flex min-w-0 flex-col">
        <span className="font-medium wrap-anywhere">{name}</span>
        <span className="text-xs wrap-anywhere text-muted-foreground">
          {detail}
        </span>
      </span>
    </span>
  )
}

function LevelSelect({
  label,
  value,
  options,
  disabled,
  size = "sm",
  onChange,
}: {
  label: string
  size?: "sm" | "default"
  value: Level
  options: ReturnType<typeof levelOptions>
  disabled: boolean
  onChange: (level: Level) => void
}) {
  return (
    <Select
      items={options.map(({ level }) => ({
        value: level,
        label: levelLabels[level],
      }))}
      value={value}
      disabled={disabled}
      onValueChange={(next) => {
        if (next !== null && next !== value) onChange(next)
      }}
    >
      <SelectTrigger size={size} className="w-28" aria-label={label}>
        <SelectValue />
      </SelectTrigger>
      <SelectContent
        alignItemWithTrigger={false}
        align="start"
        className="w-64 max-w-[calc(100vw-2rem)]"
      >
        {options.map(({ level, disabledReason }) => (
          <SelectItem
            key={level}
            value={level}
            disabled={disabledReason !== null}
          >
            <span className="flex flex-col py-0.5">
              <span>{levelLabels[level]}</span>
              {disabledReason === null ? null : (
                <span className="text-xs whitespace-normal text-muted-foreground">
                  {disabledReason}
                </span>
              )}
            </span>
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  )
}

function AddGrantForm({
  inputRef,
  listedEmails,
  pending,
  onAdd,
}: {
  inputRef: Ref<HTMLInputElement>
  listedEmails: ReadonlySet<string>
  pending: boolean
  onAdd: (change: GrantChange, onDone: () => void) => void
}) {
  const instance = useQuery(instanceQuery())
  const { principal } = useRouteContext({ from: "/_authed" })
  const mayListPeople = roleAtLeast(principal.role, "admin")
  const people = useQuery({ ...usersQuery(), enabled: mayListPeople })
  const suggestions = mayListPeople
    ? (people.data ?? [])
        .map((person) => person.email)
        .filter((email) => !listedEmails.has(email.toLowerCase()))
    : null
  const [draft, setDraft] = useState("")
  const [level, setLevel] = useState<Level>("view")
  const [draftError, setDraftError] = useState<string | null>(null)
  const errorId = useId()
  const provider = instance.data?.oidc_provider_name || "your identity provider"
  const parsed = parseGrantee(draft)
  const options = levelOptions("error" in parsed ? { group: draft } : parsed)

  function submit(event: FormEvent) {
    event.preventDefault()
    if (pending) return
    const grantee = parseGrantee(draft)
    if ("error" in grantee) {
      setDraftError(grantee.error)
      return
    }
    setDraftError(null)
    onAdd({ kind: "add", grantee, level }, () => {
      setDraft("")
      setLevel("view")
    })
  }

  return (
    <form
      onSubmit={submit}
      className="flex flex-wrap items-start gap-2 border-b px-4 py-3"
    >
      <div className="min-w-[240px] flex-1">
        <GranteeInput
          value={draft}
          onChange={(next) => {
            setDraft(next)
            setDraftError(null)
          }}
          suggestions={suggestions}
          inputRef={inputRef}
          placeholder={`Email or group name from ${provider}`}
          invalid={draftError !== null}
          describedBy={draftError === null ? undefined : errorId}
        />
        {draftError === null ? null : (
          <p id={errorId} className="mt-1 text-xs text-destructive">
            {draftError}
          </p>
        )}
      </div>
      <LevelSelect
        label="Access for the new person or group"
        value={level}
        options={options}
        disabled={pending}
        size="default"
        onChange={setLevel}
      />
      <Button
        type="submit"
        disabled={draft.trim() === "" || pending}
        aria-busy={pending}
      >
        {pending ? (
          <LoaderCircleIcon
            aria-hidden="true"
            className="motion-safe:animate-spin"
          />
        ) : (
          <PlusIcon aria-hidden="true" />
        )}
        Add
      </Button>
    </form>
  )
}
