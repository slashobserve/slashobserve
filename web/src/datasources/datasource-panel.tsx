import { useEffect, useId, useRef, useState, type RefObject } from "react"
import { useQuery, useQueryClient } from "@tanstack/react-query"
import { LoaderCircleIcon } from "lucide-react"
import { toast } from "sonner"

import { datasourceKeys } from "@/api/keys"
import { LoadProblem } from "@/components/load-problem"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { SidePanel } from "@/shell/side-panel"
import { loadProblemText } from "@/service-accounts/queries"

import {
  datasourceConfigQuery,
  DatasourceError,
  useCreateDatasource,
  useTestDatasource,
  useUpdateDatasource,
} from "./admin-queries"
import { DatasourceForm } from "./datasource-form"
import {
  draftOf,
  emptyForm,
  formOf,
  noCredentials,
  savedCredentials,
  splitProblems,
  validate,
  type DatasourceType,
  type FormValues,
  type Problems,
  type SavedCredentials,
} from "./draft"
import { failureAdvice, rowLabel } from "./format"
import type { DatasourceStatus } from "./queries"
import { saveProblemText } from "./problems"
import { RestrictedHolders } from "./restricted-holders"

type Target =
  { mode: "create"; type?: DatasourceType } | { mode: "edit"; uid: string }

type Props = {
  target: Target
  onClose: () => void
  trigger: RefObject<HTMLElement | null>
  onPickMcp: () => void
}

export function DatasourcePanel({
  target,
  onClose,
  trigger,
  onPickMcp,
}: Props) {
  if (target.mode === "create") {
    return (
      <Editor
        target={target}
        initial={{ ...emptyForm, type: target.type ?? "" }}
        saved={noCredentials}
        onClose={onClose}
        trigger={trigger}
        onPickMcp={onPickMcp}
      />
    )
  }
  return (
    <EditLoader
      uid={target.uid}
      onClose={onClose}
      trigger={trigger}
      onPickMcp={onPickMcp}
    />
  )
}

function EditLoader({
  uid,
  onClose,
  trigger,
  onPickMcp,
}: {
  uid: string
  onClose: () => void
  trigger: RefObject<HTMLElement | null>
  onPickMcp: () => void
}) {
  const config = useQuery(datasourceConfigQuery(uid))
  if (config.data !== undefined) {
    return (
      <Editor
        target={{ mode: "edit", uid }}
        initial={formOf(config.data)}
        saved={savedCredentials(config.data.secrets_set)}
        onClose={onClose}
        trigger={trigger}
        onPickMcp={onPickMcp}
      />
    )
  }
  return (
    <SidePanel
      title="Edit datasource"
      panelKey={panelKeyOf(uid)}
      onClose={onClose}
      trigger={trigger}
      footer={
        <Button variant="outline" onClick={onClose}>
          Cancel
        </Button>
      }
    >
      {config.status === "pending" ? (
        <div className="flex flex-col gap-3" aria-busy="true">
          <Skeleton className="h-8 w-full" />
          <Skeleton className="h-8 w-full" />
          <Skeleton className="h-8 w-full" />
        </div>
      ) : (
        <LoadProblem
          title="Could not load this datasource"
          detail={loadProblemText(config.error)}
          retrying={config.isFetching}
          onRetry={() => void config.refetch()}
        />
      )}
    </SidePanel>
  )
}

function panelKeyOf(uid: string) {
  return uid === ""
    ? "side-panel:datasource:new"
    : `side-panel:datasource:${uid}`
}

type TestOutcome =
  | { kind: "healthy" }
  | {
      kind: "unhealthy"
      message: string
      detail: string | undefined
      advice: string | undefined
    }
  | { kind: "failed"; message: string }

function Editor({
  target,
  initial,
  saved,
  onClose,
  trigger,
  onPickMcp,
}: Props & {
  initial: FormValues
  saved: SavedCredentials
}) {
  const formId = useId()
  const firstInput = useRef<HTMLElement>(null)
  const mode = target.mode
  const uid = target.mode === "edit" ? target.uid : ""
  const [values, setValues] = useState(initial)
  const [problems, setProblems] = useState<Problems>({})
  const [general, setGeneral] = useState<string[]>([])
  const [outcome, setOutcome] = useState<TestOutcome | null>(null)
  const create = useCreateDatasource()
  const update = useUpdateDatasource(uid)
  const test = useTestDatasource()
  const save = mode === "create" ? create : update
  const busy = save.isPending || test.isPending
  const feedback = useRef<HTMLDivElement>(null)
  const listed =
    useQueryClient().getQueryData<{ datasources: DatasourceStatus[] }>(
      datasourceKeys.list()
    )?.datasources ?? []
  const label = rowLabel(
    { uid, name: initial.name === "" ? uid : initial.name },
    listed
  )

  function change(next: Partial<FormValues>) {
    setValues((current) => ({ ...current, ...next }))
    setOutcome(null)
    if (next.audience !== undefined) save.reset()
  }

  function showServerProblem(error: unknown, type: FormValues["type"]) {
    if (
      error instanceof DatasourceError &&
      error.code === "invalid_datasource"
    ) {
      const split = splitProblems(error.fields, type, mode)
      setProblems(split.inline)
      setGeneral(split.general)
      return
    }
    if (error instanceof DatasourceError && error.code === "uid_taken") {
      setProblems({ uid: "Another datasource already uses this id." })
    }
  }

  function check(): boolean {
    const found = validate(values, mode)
    setProblems(found)
    setGeneral([])
    return Object.keys(found).length === 0
  }

  function submit(event: React.FormEvent) {
    event.preventDefault()
    if (busy || !check()) return
    const draft = draftOf(values, mode)
    const done = {
      onSuccess: () => {
        toast.success(
          mode === "create" ? `Added ${values.uid.trim()}` : `Saved ${label}`
        )
        onClose()
      },
      onError: (error: Error) => showServerProblem(error, values.type),
    }
    if (mode === "create") create.mutate(draft, done)
    else update.mutate(draft, done)
  }

  function runTest() {
    if (busy || !check()) return
    setOutcome(null)
    test.mutate(
      {
        draft: draftOf(values, mode),
        base: mode === "edit" ? uid : undefined,
      },
      {
        onSuccess: (result) =>
          setOutcome(
            result.healthy
              ? { kind: "healthy" }
              : {
                  kind: "unhealthy",
                  message: result.message ?? "The datasource did not answer.",
                  detail: result.detail,
                  advice: failureAdvice(result.failure_kind),
                }
          ),
        onError: (error) => {
          showServerProblem(error, values.type)
          setOutcome({ kind: "failed", message: saveProblemText(error) })
        },
      }
    )
  }

  const refusal =
    save.error instanceof DatasourceError &&
    save.error.code === "restricted_content"
      ? save.error
      : null
  const saveError =
    save.isError && refusal === null ? saveProblemText(save.error) : null
  const hasFeedback =
    outcome !== null ||
    refusal !== null ||
    saveError !== null ||
    general.length > 0

  useEffect(() => {
    if (hasFeedback) feedback.current?.scrollIntoView?.({ block: "nearest" })
  }, [hasFeedback, outcome, saveError, refusal, general])

  return (
    <SidePanel
      title={mode === "create" ? createTitle(values.type) : `Edit ${label}`}
      panelKey={panelKeyOf(mode === "edit" ? uid : "")}
      description={
        mode === "create"
          ? "The server connects to it with the address and credentials you give, and only an Admin can change them."
          : undefined
      }
      onClose={onClose}
      closeDisabled={save.isPending}
      initialFocus={firstInput}
      trigger={trigger}
      footer={
        <>
          <Button
            type="submit"
            form={formId}
            disabled={busy}
            aria-busy={save.isPending}
          >
            {save.isPending ? (
              <LoaderCircleIcon
                aria-hidden="true"
                className="motion-safe:animate-spin"
              />
            ) : null}
            {mode === "create" ? "Add datasource" : "Save changes"}
          </Button>
          <Button
            variant="outline"
            disabled={busy}
            aria-busy={test.isPending}
            onClick={runTest}
          >
            {test.isPending ? (
              <LoaderCircleIcon
                aria-hidden="true"
                className="motion-safe:animate-spin"
              />
            ) : null}
            Test connection
          </Button>
          <Button variant="outline" onClick={onClose} disabled={save.isPending}>
            Cancel
          </Button>
        </>
      }
    >
      <form
        id={formId}
        noValidate
        onSubmit={submit}
        className="flex flex-col gap-4"
      >
        <DatasourceForm
          mode={mode}
          values={values}
          problems={problems}
          saved={saved}
          onChange={change}
          onPickMcp={onPickMcp}
          firstInputRef={firstInput}
        />
        <div ref={feedback} aria-live="polite" className="flex flex-col gap-2">
          {outcome === null ? null : <TestResult outcome={outcome} />}
          {refusal === null ? null : <RestrictedHolders error={refusal} />}
          {saveError === null ? null : (
            <Alert variant="destructive">
              <AlertDescription>{saveError}</AlertDescription>
            </Alert>
          )}
          {general.length === 0 ? null : (
            <Alert variant="destructive">
              <AlertDescription>
                <ul className="list-disc pl-4">
                  {general.map((line) => (
                    <li key={line}>{line}</li>
                  ))}
                </ul>
              </AlertDescription>
            </Alert>
          )}
        </div>
      </form>
    </SidePanel>
  )
}

function createTitle(type: FormValues["type"]) {
  return type === "" ? "Add a source" : "Add a datasource"
}

function TestResult({ outcome }: { outcome: TestOutcome }) {
  if (outcome.kind === "healthy") {
    return (
      <p className="text-[13px] text-status-ok">
        Connected. Save to keep these settings.
      </p>
    )
  }
  if (outcome.kind === "failed") {
    return (
      <Alert variant="destructive">
        <AlertDescription>{outcome.message}</AlertDescription>
      </Alert>
    )
  }
  return (
    <Alert variant="destructive">
      <AlertDescription>
        <p>{outcome.message}</p>
        {outcome.detail === undefined ? null : <p>{outcome.detail}</p>}
        {outcome.advice === undefined ? null : <p>{outcome.advice}</p>}
      </AlertDescription>
    </Alert>
  )
}
