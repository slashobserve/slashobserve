import { useId, useRef, useState, type FormEvent, type RefObject } from "react"
import { useQuery } from "@tanstack/react-query"
import { Link, useNavigate } from "@tanstack/react-router"
import {
  ChevronDownIcon,
  LayoutDashboardIcon,
  LoaderCircleIcon,
  SparklesIcon,
} from "lucide-react"
import { toast } from "sonner"

import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLinkItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { foldersQuery } from "@/folders/queries"
import { dashboardLink } from "@/lib/links"
import { useAgentInfo } from "@/observe/queries"

import { blankDashboardSpec, blankDashboardTargets } from "./blank-dashboard"
import { titleError } from "./edit/details"
import { saveProblemFrom, type Problem } from "./edit/save-problem"
import { useCreateDashboard } from "./queries"

export function CreateDashboardMenu() {
  const agent = useAgentInfo()
  const needsModel = agent.data?.available === false
  const trigger = useRef<HTMLButtonElement>(null)
  const [blank, setBlank] = useState(false)
  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger render={<Button ref={trigger} />}>
          Create dashboard
          <ChevronDownIcon aria-hidden="true" />
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="w-60">
          <DropdownMenuLinkItem render={<Link to="/ask" />}>
            <SparklesIcon aria-hidden="true" />
            <span className="flex-1">Ask the agent</span>
            {needsModel ? (
              <span className="text-xs text-muted-foreground">
                needs a model
              </span>
            ) : null}
          </DropdownMenuLinkItem>
          <DropdownMenuItem onClick={() => setBlank(true)}>
            <LayoutDashboardIcon aria-hidden="true" />
            Blank dashboard
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
      <BlankDashboardDialog
        open={blank}
        onOpenChange={setBlank}
        finalFocus={trigger}
      />
    </>
  )
}

const rejectedHeading = "The server rejected this dashboard:"

function BlankDashboardDialog({
  open,
  onOpenChange,
  finalFocus,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  finalFocus: RefObject<HTMLButtonElement | null>
}) {
  const navigate = useNavigate()
  const create = useCreateDashboard()
  const folders = useQuery({ ...foldersQuery(), enabled: open })
  const targets = blankDashboardTargets(folders.data ?? [])
  const ids = useId()
  const titleId = `${ids}-title`
  const folderId = `${ids}-folder`
  const titleRef = useRef<HTMLInputElement>(null)
  const opened = useRef(false)
  const [title, setTitle] = useState("")
  const [folderEdit, setFolderEdit] = useState<string>()
  const [submitted, setSubmitted] = useState(false)
  const [problem, setProblem] = useState<Problem | null>(null)

  const folder = folderEdit ?? targets.initial
  const titleProblem = titleError(title)
  const shownTitleProblem = submitted ? titleProblem : null
  const items = targets.choices.map((choice) => ({
    value: choice.id,
    label: choice.title,
  }))
  const noFolder = folders.isSuccess && folder === null

  function close() {
    onOpenChange(false)
    setTitle("")
    setFolderEdit(undefined)
    setSubmitted(false)
    setProblem(null)
    create.reset()
  }

  function changeOpen(next: boolean) {
    if (next) onOpenChange(true)
    else if (!create.isPending) close()
  }

  function submit(event: FormEvent) {
    event.preventDefault()
    setSubmitted(true)
    if (titleProblem !== null || folder === null || create.isPending) return
    setProblem(null)
    const spec = blankDashboardSpec(title)
    const folderTitle = items.find((item) => item.value === folder)?.label
    create.mutate(
      { spec, folderId: folder },
      {
        onSuccess: (created) => {
          opened.current = true
          toast.success(`Created "${spec.title}" in ${folderTitle}`)
          close()
          void navigate(dashboardLink(created.dashboard.id, spec.title))
        },
        onError: (failure) =>
          setProblem(saveProblemFrom(failure, rejectedHeading)),
      }
    )
  }

  return (
    <Dialog open={open} onOpenChange={changeOpen}>
      <DialogContent
        className="sm:max-w-md"
        initialFocus={titleRef}
        finalFocus={() => {
          const leaveFocus = opened.current
          opened.current = false
          return leaveFocus ? false : (finalFocus.current ?? true)
        }}
      >
        <form className="grid gap-4" onSubmit={submit} noValidate>
          <DialogHeader>
            <DialogTitle>Blank dashboard</DialogTitle>
            <DialogDescription>
              Creates a dashboard with one note on how to add panels. Add panels
              from its More menu.
            </DialogDescription>
          </DialogHeader>
          <div className="grid gap-2">
            <Label htmlFor={titleId}>Title</Label>
            <Input
              id={titleId}
              ref={titleRef}
              autoComplete="off"
              value={title}
              readOnly={create.isPending}
              aria-invalid={shownTitleProblem !== null}
              aria-describedby={
                shownTitleProblem !== null ? `${titleId}-error` : undefined
              }
              onChange={(event) => setTitle(event.target.value)}
            />
            {shownTitleProblem !== null ? (
              <p
                id={`${titleId}-error`}
                role="alert"
                className="text-sm text-destructive"
              >
                {shownTitleProblem}
              </p>
            ) : null}
          </div>
          <div className="grid gap-2">
            <Label htmlFor={folderId}>Folder</Label>
            <Select
              items={items}
              value={folder}
              disabled={create.isPending || !folders.isSuccess}
              onValueChange={(next) => setFolderEdit(next ?? undefined)}
            >
              <SelectTrigger id={folderId} className="w-full">
                <SelectValue
                  placeholder={
                    folders.isError ? "Folders did not load" : "Loading folders"
                  }
                />
              </SelectTrigger>
              <SelectContent alignItemWithTrigger={false} align="start">
                {items.map((item) => (
                  <SelectItem key={item.value} value={item.value}>
                    {item.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            {noFolder ? (
              <p className="text-sm text-muted-foreground">
                You cannot create dashboards in any folder.
              </p>
            ) : null}
          </div>
          {folders.isError ? (
            <Alert variant="destructive">
              <AlertDescription className="flex flex-wrap items-center gap-2">
                The folders could not be loaded.
                <Button
                  type="button"
                  size="sm"
                  variant="outline"
                  onClick={() => void folders.refetch()}
                >
                  Retry
                </Button>
              </AlertDescription>
            </Alert>
          ) : null}
          {problem?.kind === "invalid" ? (
            <Alert variant="destructive">
              <AlertDescription>
                <p>{problem.heading}</p>
                <ul className="list-disc pl-4">
                  {problem.lines.map((line) => (
                    <li key={line}>{line}</li>
                  ))}
                </ul>
              </AlertDescription>
            </Alert>
          ) : null}
          {problem?.kind === "failed" ? (
            <Alert variant="destructive">
              <AlertDescription>{problem.message}</AlertDescription>
            </Alert>
          ) : null}
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              aria-disabled={create.isPending}
              onClick={() => changeOpen(false)}
            >
              Cancel
            </Button>
            <Button
              type="submit"
              focusableWhenDisabled
              disabled={create.isPending || folder === null}
              aria-busy={create.isPending}
            >
              {create.isPending ? (
                <LoaderCircleIcon
                  aria-hidden="true"
                  className="motion-safe:animate-spin"
                />
              ) : null}
              Create dashboard
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
