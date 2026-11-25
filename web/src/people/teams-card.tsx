import { useId, useRef, useState, type FormEvent } from "react"
import { useQuery } from "@tanstack/react-query"
import { LoaderCircleIcon, PlusIcon, XIcon } from "lucide-react"
import { toast } from "sonner"

import { LoadProblem } from "@/components/load-problem"
import { DataGrid, type DataGridColumn } from "@/components/data-grid/data-grid"
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
import {
  Combobox,
  ComboboxContent,
  ComboboxEmpty,
  ComboboxInput,
  ComboboxItem,
  ComboboxList,
  ComboboxTrigger,
} from "@/components/ui/combobox"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Skeleton } from "@/components/ui/skeleton"
import { loadProblemText } from "@/service-accounts/queries"

import { usersQuery, type User } from "./queries"
import {
  candidatesFor,
  teamNameProblem,
  teamProblemText,
  teamsQuery,
  useCreateTeam,
  useDeleteTeam,
  useTeamMember,
  type Team,
} from "./teams"

function teamId(team: Team): string {
  return team.id
}

function personLabel(person: { name: string; email: string }): string {
  return person.name === "" ? person.email : person.name
}

function NewTeamButton() {
  const trigger = useRef<HTMLButtonElement>(null)
  const inputId = useId()
  const create = useCreateTeam()
  const [open, setOpen] = useState(false)
  const [name, setName] = useState("")
  const [problem, setProblem] = useState<string | null>(null)

  function close() {
    setOpen(false)
    setName("")
    setProblem(null)
    create.reset()
  }

  function submit(event: FormEvent) {
    event.preventDefault()
    if (create.isPending) return
    const found = teamNameProblem(name)
    setProblem(found)
    if (found !== null) return
    create.mutate(name, {
      onSuccess: (team) => {
        toast.success(`Created team ${team.name}`)
        close()
      },
      onError: (error) => setProblem(teamProblemText(error)),
    })
  }

  return (
    <>
      <Button ref={trigger} variant="outline" onClick={() => setOpen(true)}>
        <PlusIcon aria-hidden="true" />
        New team
      </Button>
      <Dialog
        open={open}
        onOpenChange={(next) => {
          if (next) setOpen(true)
          else if (!create.isPending) close()
        }}
      >
        <DialogContent finalFocus={trigger}>
          <form noValidate onSubmit={submit} className="grid gap-4">
            <DialogHeader>
              <DialogTitle>New team</DialogTitle>
              <DialogDescription>
                Share a folder with the team by adding its name as a group in
                the folder&apos;s access list.
              </DialogDescription>
            </DialogHeader>
            <div className="flex flex-col gap-1.5">
              <Label htmlFor={inputId}>Name</Label>
              <Input
                id={inputId}
                autoComplete="off"
                maxLength={100}
                className="narrow:h-11"
                value={name}
                aria-invalid={problem !== null}
                onChange={(event) => {
                  setName(event.target.value)
                  setProblem(null)
                }}
              />
            </div>
            {problem === null ? null : (
              <Alert variant="destructive">
                <AlertDescription>{problem}</AlertDescription>
              </Alert>
            )}
            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                disabled={create.isPending}
                onClick={close}
              >
                Cancel
              </Button>
              <Button type="submit" aria-busy={create.isPending}>
                {create.isPending ? (
                  <LoaderCircleIcon
                    aria-hidden="true"
                    className="motion-safe:animate-spin"
                  />
                ) : null}
                Create team
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </>
  )
}

function AddMember({ team, people }: { team: Team; people: readonly User[] }) {
  const member = useTeamMember()
  const [query, setQuery] = useState("")
  const options = candidatesFor(team, people)

  function add(person: User) {
    member.mutate(
      { team: team.id, user: person.id, add: true },
      {
        onSuccess: () =>
          toast.success(`Added ${personLabel(person)} to ${team.name}`),
        onError: (error) => toast.error(teamProblemText(error)),
      }
    )
  }

  return (
    <Combobox<User>
      items={options}
      value={null}
      inputValue={query}
      disabled={member.isPending || options.length === 0}
      itemToStringLabel={(person) => `${personLabel(person)} ${person.email}`}
      onOpenChange={(next) => {
        if (!next) setQuery("")
      }}
      onInputValueChange={(text, details) =>
        setQuery(details.reason === "item-press" ? "" : text)
      }
      onValueChange={(next) => {
        if (next !== null) add(next)
      }}
    >
      <ComboboxTrigger
        aria-label={`Add a person to ${team.name}`}
        className="[&>svg:last-child]:hidden"
        render={<Button variant="outline" size="sm" className="narrow:h-11" />}
      >
        Add person
      </ComboboxTrigger>
      <ComboboxContent className="min-w-64">
        <ComboboxInput
          aria-label="Search people"
          placeholder="Search people"
          showTrigger={false}
        />
        <ComboboxEmpty>No one found</ComboboxEmpty>
        <ComboboxList>
          {(person: User) => (
            <ComboboxItem key={person.id} value={person}>
              <span className="truncate">{personLabel(person)}</span>
              <span className="ml-auto truncate text-xs text-muted-foreground">
                {person.email}
              </span>
            </ComboboxItem>
          )}
        </ComboboxList>
      </ComboboxContent>
    </Combobox>
  )
}

function Members({ team }: { team: Team }) {
  const member = useTeamMember()
  if (team.members.length === 0) {
    return <span className="text-muted-foreground">No members yet</span>
  }
  return (
    <ul className="flex flex-wrap gap-1.5">
      {team.members.map((person) => (
        <li
          key={person.id}
          className="inline-flex items-center gap-1 rounded-control border py-0.5 pr-0.5 pl-2 text-xs"
        >
          <span className="wrap-anywhere">{personLabel(person)}</span>
          <Button
            variant="ghost"
            size="icon-xs"
            className="narrow:size-9"
            disabled={member.isPending}
            aria-label={`Remove ${personLabel(person)} from ${team.name}`}
            onClick={() =>
              member
                .mutateAsync({ team: team.id, user: person.id, add: false })
                .then(
                  () =>
                    toast.success(
                      `Removed ${personLabel(person)} from ${team.name}`
                    ),
                  (error) => toast.error(teamProblemText(error))
                )
            }
          >
            <XIcon aria-hidden="true" />
          </Button>
        </li>
      ))}
    </ul>
  )
}

function DeleteTeamButton({ team }: { team: Team }) {
  const [open, setOpen] = useState(false)
  const remove = useDeleteTeam()

  function changeOpen(next: boolean) {
    if (!next && remove.isPending) return
    setOpen(next)
    if (!next) remove.reset()
  }

  return (
    <>
      <Button
        variant="outline"
        size="sm"
        className="narrow:h-11"
        aria-label={`Delete team ${team.name}`}
        onClick={() => setOpen(true)}
      >
        Delete
      </Button>
      <AlertDialog open={open} onOpenChange={changeOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle className="wrap-anywhere">
              Delete team {team.name}?
            </AlertDialogTitle>
            <AlertDialogDescription>
              Its members lose the folder access the name {team.name} gave them.
              Folder entries for the group stay and match no one unless your
              identity provider sends a group with that name.
            </AlertDialogDescription>
          </AlertDialogHeader>
          {remove.isError ? (
            <Alert variant="destructive">
              <AlertDescription>
                {teamProblemText(remove.error)}
              </AlertDescription>
            </Alert>
          ) : null}
          <AlertDialogFooter>
            <AlertDialogCancel aria-disabled={remove.isPending}>
              Cancel
            </AlertDialogCancel>
            <AlertDialogAction
              variant="destructive"
              disabled={remove.isPending}
              focusableWhenDisabled
              aria-busy={remove.isPending}
              onClick={() =>
                remove.mutateAsync(team.id).then(
                  () => {
                    toast.success(`Deleted team ${team.name}`)
                    setOpen(false)
                  },
                  () => {}
                )
              }
            >
              {remove.isPending ? (
                <LoaderCircleIcon
                  aria-hidden="true"
                  className="motion-safe:animate-spin"
                />
              ) : null}
              Delete team
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  )
}

function teamColumns(people: readonly User[]): DataGridColumn<Team>[] {
  return [
    {
      id: "team",
      header: "Team",
      minWidth: 140,
      autoHeight: true,
      cell: (team) => (
        <span className="py-2 font-medium wrap-anywhere">{team.name}</span>
      ),
    },
    {
      id: "members",
      header: "Members",
      minWidth: 240,
      flex: 2,
      autoHeight: true,
      cell: (team) => (
        <span className="py-2">
          <Members team={team} />
        </span>
      ),
    },
    {
      id: "actions",
      header: "Actions",
      action: true,
      align: "right",
      width: 260,
      cell: (team) => (
        <span className="inline-flex items-center justify-end gap-2">
          <AddMember team={team} people={people} />
          <DeleteTeamButton team={team} />
        </span>
      ),
    },
  ]
}

function TeamsBody() {
  const teams = useQuery(teamsQuery())
  const people = useQuery(usersQuery())

  if (teams.isPending) {
    return <Skeleton className="h-10 w-full" aria-busy="true" />
  }
  if (teams.data === undefined) {
    return (
      <LoadProblem
        title="Could not load teams"
        detail={loadProblemText(teams.error)}
        retrying={teams.isFetching}
        onRetry={() => void teams.refetch()}
      />
    )
  }
  if (teams.data.length === 0) {
    return (
      <p className="rounded-card border px-4 py-6 text-center text-sm text-muted-foreground">
        No teams yet. A team gives its members the folder access granted to its
        name.
      </p>
    )
  }
  return (
    <DataGrid
      label="Teams"
      rows={teams.data}
      columns={teamColumns(people.data ?? [])}
      rowId={teamId}
      rowAttributes={(team) => ({ "data-team-id": team.id })}
    />
  )
}

export function TeamsCard() {
  const headingId = useId()
  return (
    <section aria-labelledby={headingId} className="mt-8 flex flex-col gap-2">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div className="flex flex-col">
          <h2 id={headingId} className="text-sm font-semibold">
            Teams
          </h2>
          <p className="text-[13px] text-muted-foreground">
            Add a team&apos;s name as a group in a folder&apos;s access list to
            share the folder with its members. Teams never change a role.
          </p>
        </div>
        <NewTeamButton />
      </div>
      <TeamsBody />
    </section>
  )
}
