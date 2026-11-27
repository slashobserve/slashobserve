import {
  useEffect,
  useId,
  useReducer,
  useRef,
  type FormEvent,
  type RefObject,
} from "react"
import { useQuery } from "@tanstack/react-query"
import { LoaderCircleIcon } from "lucide-react"

import { CopyButton } from "@/components/copy-button"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Switch } from "@/components/ui/switch"
import { roleDescriptions, roleLabels, type Role } from "@/lib/roles"
import { RoleSelect } from "@/people/role-select"
import { SidePanel } from "@/shell/side-panel"

import { accountChanges } from "./account-changes"
import {
  accountWithoutKey,
  emptyKeyPanel,
  reduceKeyPanel,
} from "./create-key-state"
import {
  accountNameError,
  expiresAtFor,
  expiryOptions,
  keyNameError,
} from "./key-status"
import {
  accountProblemText,
  serviceAccountsQuery,
  useCreateKey,
  useCreateServiceAccount,
  useUpdateServiceAccount,
} from "./queries"

type Props = {
  onClose: () => void
  nameInput: RefObject<HTMLInputElement | null>
  copyButton: RefObject<HTMLButtonElement | null>
}

const newAccountChoice = "new"

const newAccountRole: Role = "editor"

const copyButtonClass = "px-2 [&_svg:not([class*='size-'])]:size-[13px]"

const expiryItems = expiryOptions.map(({ value, label }) => ({ value, label }))

function FieldError({ id, text }: { id: string; text: string | null }) {
  if (text === null) return null
  return (
    <p id={id} role="alert" className="text-sm text-destructive">
      {text}
    </p>
  )
}

export function CreateKeyPanel({ onClose, nameInput, copyButton }: Props) {
  const accountsQuery = useQuery(serviceAccountsQuery())
  const createAccount = useCreateServiceAccount()
  const updateAccount = useUpdateServiceAccount()
  const createKey = useCreateKey()
  const ids = useId()
  const newAccountInput = useRef<HTMLInputElement>(null)

  const [state, dispatch] = useReducer(reduceKeyPanel, emptyKeyPanel)
  const {
    step,
    keyName,
    accountChoice,
    newAccountName,
    roleChoice,
    takesRunsChoice,
    expiry,
    submitted,
    createdAccount,
  } = state

  const loaded = accountsQuery.data ?? []
  const accounts = (
    createdAccount !== null &&
    !loaded.some((account) => account.id === createdAccount.id)
      ? [...loaded, createdAccount]
      : loaded
  ).toSorted((a, b) => a.name.localeCompare(b.name))
  const choice =
    accountChoice ?? (accountsQuery.isPending ? null : newAccountChoice)
  const accountItems = [
    ...accounts.map((account) => ({ value: account.id, label: account.name })),
    { value: newAccountChoice, label: "New service account" },
  ]
  const creating = choice === newAccountChoice
  const loadingAccounts = choice === null
  const locked = step.phase === "issued"
  const pending = step.phase === "creating"
  const keyless = accountWithoutKey(state)
  const problem =
    step.phase === "form" || step.phase === "partly_done" ? step.problem : null
  const chosenAccount = accounts.find((account) => account.id === choice)
  const accountRole = creating
    ? newAccountRole
    : (chosenAccount?.role ?? newAccountRole)
  const role = roleChoice ?? accountRole
  const accountTakesRuns = creating
    ? false
    : (chosenAccount?.takes_runs ?? false)
  const takesRuns = takesRunsChoice ?? accountTakesRuns
  const takesRunsLocked = !creating && chosenAccount?.provisioned === true

  useEffect(() => {
    if (step.phase === "issued") copyButton.current?.focus()
  }, [step.phase, copyButton])

  const keyNameProblem = keyNameError(keyName)
  const accountNameProblem = creating ? accountNameError(newAccountName) : null
  const showKeyNameError =
    keyNameProblem !== null && (submitted || keyName.length > 0)
  const showAccountNameError =
    accountNameProblem !== null && (submitted || newAccountName.length > 0)

  async function submit(event: FormEvent) {
    event.preventDefault()
    if (pending || locked || loadingAccounts) return
    if (keyNameProblem !== null) {
      dispatch({ type: "submit_refused" })
      nameInput.current?.focus()
      return
    }
    if (accountNameProblem !== null) {
      dispatch({ type: "submit_refused" })
      newAccountInput.current?.focus()
      return
    }
    dispatch({ type: "submit_started" })
    let accountId = choice
    let accountName = chosenAccount?.name ?? ""
    if (creating) {
      try {
        const account = await createAccount.mutateAsync({
          name: newAccountName,
          role,
        })
        accountId = account.id
        accountName = account.name
        dispatch({ type: "account_created", account })
        if (takesRuns) {
          const updated = await updateAccount.mutateAsync({
            id: account.id,
            takes_runs: true,
          })
          dispatch({ type: "account_updated", account: updated })
        }
      } catch (error) {
        dispatch({
          type: "failed",
          problem: accountProblemText(error, {
            accountName: newAccountName,
            what: "account",
          }),
        })
        return
      }
    } else if (chosenAccount !== undefined) {
      const changes = accountChanges(chosenAccount, { role, takesRuns })
      try {
        if (changes !== null) {
          await updateAccount.mutateAsync({ id: chosenAccount.id, ...changes })
        }
      } catch (error) {
        dispatch({
          type: "failed",
          problem: accountProblemText(error, { what: "account" }),
        })
        return
      }
    }
    createKey.mutate(
      { accountId, name: keyName.trim(), expires_at: expiresAtFor(expiry) },
      {
        onSuccess: (response) =>
          dispatch({
            type: "key_issued",
            token: response.token,
            accountName,
            role,
          }),
        onError: (error) =>
          dispatch({
            type: "failed",
            problem: accountProblemText(error, { what: "key" }),
          }),
      }
    )
  }

  const keyNameErrorId = `${ids}-key-name-error`
  const keyNameHintId = `${ids}-key-name-hint`
  const accountNameErrorId = `${ids}-account-name-error`
  const accountNameHintId = `${ids}-account-name-hint`
  const formId = `${ids}-form`
  const newKeyLabelId = `${ids}-new-key-label`

  return (
    <SidePanel
      title="Create key"
      description="The key is shown once. Copy it before you close this panel."
      initialFocus={nameInput}
      closeDisabled={pending}
      onClose={onClose}
      footer={
        locked ? (
          <Button variant="outline" onClick={onClose}>
            Done
          </Button>
        ) : (
          <>
            <Button variant="outline" disabled={pending} onClick={onClose}>
              Cancel
            </Button>
            <Button
              type="submit"
              form={formId}
              focusableWhenDisabled
              disabled={pending || loadingAccounts}
              aria-busy={pending}
            >
              {pending ? (
                <LoaderCircleIcon
                  aria-hidden="true"
                  className="size-3.5 motion-safe:animate-spin"
                />
              ) : null}
              {keyless === null ? "Create key" : "Issue the key again"}
            </Button>
          </>
        )
      }
    >
      <form
        id={formId}
        noValidate
        onSubmit={(event) => void submit(event)}
        className="flex flex-col gap-4"
      >
        <div className="flex flex-col gap-1.5">
          <Label htmlFor={`${ids}-key-name`}>Name</Label>
          <Input
            id={`${ids}-key-name`}
            ref={nameInput}
            value={keyName}
            autoComplete="off"
            spellCheck={false}
            disabled={locked}
            readOnly={pending}
            aria-invalid={showKeyNameError}
            aria-describedby={
              showKeyNameError
                ? `${keyNameHintId} ${keyNameErrorId}`
                : keyNameHintId
            }
            onChange={(event) =>
              dispatch({ type: "key_named", name: event.target.value })
            }
          />
          <p id={keyNameHintId} className="text-xs text-muted-foreground">
            Where it will be used, so you can tell keys apart.
          </p>
          <FieldError
            id={keyNameErrorId}
            text={showKeyNameError ? keyNameProblem : null}
          />
        </div>
        <div className="flex flex-col gap-1.5">
          <Label htmlFor={`${ids}-account`}>Service account</Label>
          {/* No finalFocus ref on these Selects: a ref skips Base UI's moved-focus guard and steals focus from the next Select. */}
          <Select
            items={accountItems}
            value={choice}
            disabled={locked || pending || loadingAccounts}
            onValueChange={(next) => {
              if (next !== null) {
                dispatch({ type: "account_chosen", choice: next })
              }
            }}
          >
            <SelectTrigger
              id={`${ids}-account`}
              className="w-full"
              aria-busy={loadingAccounts}
              icon={
                loadingAccounts ? (
                  <LoaderCircleIcon
                    aria-hidden="true"
                    className="size-4 text-muted-foreground motion-safe:animate-spin"
                  />
                ) : undefined
              }
            >
              <SelectValue placeholder="Loading service accounts" />
            </SelectTrigger>
            <SelectContent alignItemWithTrigger={false} align="start">
              {accountItems.map((item) => (
                <SelectItem key={item.value} value={item.value}>
                  {item.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        {creating ? (
          <div className="flex flex-col gap-1.5">
            <Label htmlFor={`${ids}-account-name`}>Service account name</Label>
            <Input
              id={`${ids}-account-name`}
              ref={newAccountInput}
              value={newAccountName}
              autoComplete="off"
              spellCheck={false}
              disabled={locked}
              readOnly={pending}
              aria-invalid={showAccountNameError}
              aria-describedby={
                showAccountNameError
                  ? `${accountNameHintId} ${accountNameErrorId}`
                  : accountNameHintId
              }
              onChange={(event) =>
                dispatch({ type: "account_named", name: event.target.value })
              }
            />
            <p id={accountNameHintId} className="text-xs text-muted-foreground">
              Lowercase letters, digits and dashes, for example ci-bot.
            </p>
            <FieldError
              id={accountNameErrorId}
              text={showAccountNameError ? accountNameProblem : null}
            />
          </div>
        ) : null}
        <div className="flex flex-col gap-1.5">
          <Label htmlFor={`${ids}-role`}>Role</Label>
          <RoleSelect
            id={`${ids}-role`}
            className="w-full"
            value={role}
            disabled={locked || pending || loadingAccounts}
            onValueChange={(next) =>
              dispatch({ type: "role_chosen", role: next })
            }
          />
          <p className="text-xs text-muted-foreground">
            {roleDescriptions[role]}
            {creating ? "" : " Applies to every key of this service account."}
          </p>
        </div>
        <div className="flex flex-col gap-1.5">
          <div className="flex items-center justify-between gap-3">
            <Label htmlFor={`${ids}-takes-runs`}>Takes agent runs</Label>
            <Switch
              id={`${ids}-takes-runs`}
              checked={takesRuns}
              disabled={locked || pending || loadingAccounts || takesRunsLocked}
              aria-describedby={`${ids}-takes-runs-hint`}
              onCheckedChange={(next) =>
                dispatch({ type: "takes_runs_chosen", takesRuns: next })
              }
            />
          </div>
          <p
            id={`${ids}-takes-runs-hint`}
            className="text-xs text-muted-foreground"
          >
            {takesRunsLocked
              ? "Set in the service-accounts file."
              : "Lets this account claim the runs assigned to it over MCP."}
            {creating ? "" : " Applies to every key of this service account."}
          </p>
        </div>
        <div className="flex flex-col gap-1.5">
          <Label htmlFor={`${ids}-expires`}>Expires</Label>
          <Select
            items={expiryItems}
            value={expiry}
            disabled={locked || pending}
            onValueChange={(next) => {
              if (next !== null) {
                dispatch({ type: "expiry_chosen", expiry: next })
              }
            }}
          >
            <SelectTrigger id={`${ids}-expires`} className="w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent alignItemWithTrigger={false} align="start">
              {expiryItems.map((item) => (
                <SelectItem key={item.value} value={item.value}>
                  {item.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      </form>
      {step.phase === "issued" ? (
        <div className="flex flex-col gap-1.5">
          <p className="text-[13px]">
            <span className="font-medium wrap-anywhere">
              {step.accountName}
            </span>
            <span className="text-muted-foreground">
              {" - Role: "}
              {roleLabels[step.role]}
            </span>
          </p>
          <span id={newKeyLabelId} className="text-[13px] font-medium">
            Your new key
          </span>
          <div className="flex items-center gap-2 rounded-[8px] border border-foreground py-2.5 pr-2.5 pl-3">
            <code className="min-w-0 flex-1 overflow-x-auto font-mono text-xs whitespace-nowrap">
              {step.token}
            </code>
            <CopyButton
              ref={copyButton}
              text={step.token}
              label="Copy key"
              variant="outline"
              size="sm"
              className={copyButtonClass}
              aria-describedby={newKeyLabelId}
            />
          </div>
          <p className="text-xs text-muted-foreground">
            Run this in your shell, then add the client from the page behind
            this panel:
          </p>
          <pre className="rounded-[8px] border px-3 py-2.5 font-mono text-xs break-all whitespace-pre-wrap">
            export OBSERVE_TOKEN={step.token}
          </pre>
        </div>
      ) : null}
      {problem !== null ? (
        <Alert variant="destructive">
          <AlertDescription>
            {problem}
            {keyless === null
              ? null
              : ` The service account ${keyless.name} was created, but it has no key yet.`}
          </AlertDescription>
        </Alert>
      ) : null}
    </SidePanel>
  )
}
