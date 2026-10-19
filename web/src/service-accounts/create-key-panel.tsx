import {
  useEffect,
  useId,
  useRef,
  useState,
  type FormEvent,
  type RefObject,
} from "react"
import { useQuery } from "@tanstack/react-query"
import { useRouteContext } from "@tanstack/react-router"
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
import { SidePanel } from "@/shell/side-panel"

import {
  accountNameError,
  defaultExpiry,
  expiresAtFor,
  expiryOptions,
  keyNameError,
  type ExpiryValue,
} from "./key-status"
import {
  problemText,
  serviceAccountsQuery,
  useCreateKey,
  useCreateServiceAccount,
  type ServiceAccount,
} from "./queries"

type Props = {
  onClose: () => void
  nameInput: RefObject<HTMLInputElement | null>
  copyButton: RefObject<HTMLButtonElement | null>
}

const newAccountChoice = "new"

const copyButtonClass = "px-2 [&_svg:not([class*='size-'])]:size-[13px]"

const expiryItems = expiryOptions.map(({ value, label }) => ({ value, label }))

function defaultChoice(accounts: ServiceAccount[], principalId: string) {
  const own = accounts.find((account) => account.created_by === principalId)
  return (own ?? accounts[0])?.id ?? newAccountChoice
}

function FieldError({ id, text }: { id: string; text: string | null }) {
  if (text === null) return null
  return (
    <p id={id} role="alert" className="text-sm text-destructive">
      {text}
    </p>
  )
}

export function CreateKeyPanel({ onClose, nameInput, copyButton }: Props) {
  const { principal } = useRouteContext({ from: "/_authed" })
  const accountsQuery = useQuery(serviceAccountsQuery())
  const createAccount = useCreateServiceAccount()
  const createKey = useCreateKey()
  const ids = useId()
  const newAccountInput = useRef<HTMLInputElement>(null)
  const accountTrigger = useRef<HTMLButtonElement>(null)
  const expiresTrigger = useRef<HTMLButtonElement>(null)

  const [phase, setPhase] = useState<"form" | "created">("form")
  const [keyName, setKeyName] = useState("")
  const [accountChoice, setAccountChoice] = useState<string | null>(null)
  const [createdAccount, setCreatedAccount] = useState<ServiceAccount | null>(
    null
  )
  const [newAccountName, setNewAccountName] = useState("")
  const [expiry, setExpiry] = useState<ExpiryValue>(defaultExpiry)
  const [submitted, setSubmitted] = useState(false)
  const [submitting, setSubmitting] = useState(false)
  const [problem, setProblem] = useState<string | null>(null)
  const [token, setToken] = useState<string | null>(null)

  const loaded = accountsQuery.data ?? []
  const accounts = (
    createdAccount !== null &&
    !loaded.some((account) => account.id === createdAccount.id)
      ? [...loaded, createdAccount]
      : loaded
  ).toSorted((a, b) => a.name.localeCompare(b.name))
  const choice =
    accountChoice ??
    (accountsQuery.isPending ? null : defaultChoice(accounts, principal.id))
  const accountItems = [
    ...accounts.map((account) => ({ value: account.id, label: account.name })),
    { value: newAccountChoice, label: "New service account" },
  ]
  const creating = choice === newAccountChoice
  const loadingAccounts = choice === null
  const locked = phase === "created"
  const pending = submitting || createAccount.isPending || createKey.isPending

  useEffect(() => {
    if (phase === "created") copyButton.current?.focus()
  }, [phase, copyButton])

  const keyNameProblem = keyNameError(keyName)
  const accountNameProblem = creating ? accountNameError(newAccountName) : null
  const showKeyNameError =
    keyNameProblem !== null && (submitted || keyName.length > 0)
  const showAccountNameError =
    accountNameProblem !== null && (submitted || newAccountName.length > 0)

  async function submit(event: FormEvent) {
    event.preventDefault()
    setSubmitted(true)
    if (pending || locked || loadingAccounts) return
    if (keyNameProblem !== null) {
      nameInput.current?.focus()
      return
    }
    if (accountNameProblem !== null) {
      newAccountInput.current?.focus()
      return
    }
    setProblem(null)
    setSubmitting(true)
    let accountId = choice
    if (creating) {
      try {
        const account = await createAccount.mutateAsync({
          name: newAccountName,
        })
        accountId = account.id
        setCreatedAccount(account)
        setAccountChoice(account.id)
      } catch (error) {
        setProblem(
          problemText(error, { accountName: newAccountName, what: "account" })
        )
        setSubmitting(false)
        return
      }
    }
    createKey.mutate(
      { accountId, name: keyName.trim(), expires_at: expiresAtFor(expiry) },
      {
        onSuccess: (response) => {
          setToken(response.token)
          setPhase("created")
          setSubmitting(false)
        },
        onError: (error) => {
          setProblem(problemText(error, { what: "key" }))
          setSubmitting(false)
        },
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
              Create key
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
            onChange={(event) => setKeyName(event.target.value)}
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
          <Select
            items={accountItems}
            value={choice}
            disabled={locked || pending || loadingAccounts}
            onValueChange={(next) => {
              if (next !== null) setAccountChoice(next)
            }}
          >
            <SelectTrigger
              id={`${ids}-account`}
              ref={accountTrigger}
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
            <SelectContent
              alignItemWithTrigger={false}
              align="start"
              finalFocus={accountTrigger}
            >
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
              onChange={(event) => setNewAccountName(event.target.value)}
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
          <Label htmlFor={`${ids}-expires`}>Expires</Label>
          <Select
            items={expiryItems}
            value={expiry}
            disabled={locked || pending}
            onValueChange={(next) => {
              if (next !== null) setExpiry(next)
            }}
          >
            <SelectTrigger
              id={`${ids}-expires`}
              ref={expiresTrigger}
              className="w-full"
            >
              <SelectValue />
            </SelectTrigger>
            <SelectContent
              alignItemWithTrigger={false}
              align="start"
              finalFocus={expiresTrigger}
            >
              {expiryItems.map((item) => (
                <SelectItem key={item.value} value={item.value}>
                  {item.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      </form>
      {token !== null ? (
        <div className="flex flex-col gap-1.5">
          <span id={newKeyLabelId} className="text-[13px] font-medium">
            Your new key
          </span>
          <div className="flex items-center gap-2 rounded-[8px] border border-brand py-2.5 pr-2.5 pl-3">
            <code className="min-w-0 flex-1 overflow-x-auto font-mono text-xs whitespace-nowrap">
              {token}
            </code>
            <CopyButton
              ref={copyButton}
              text={token}
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
            export OBSERVE_TOKEN={token}
          </pre>
        </div>
      ) : null}
      {problem !== null ? (
        <Alert variant="destructive">
          <AlertDescription>{problem}</AlertDescription>
        </Alert>
      ) : null}
    </SidePanel>
  )
}
