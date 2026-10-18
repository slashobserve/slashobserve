import {
  useEffect,
  useId,
  useRef,
  useState,
  type FormEvent,
  type RefObject,
} from "react"
import { useQuery, useQueryClient } from "@tanstack/react-query"
import { LoaderCircleIcon } from "lucide-react"

import { ApiError } from "@/api/errors"
import { serviceAccountKeys } from "@/api/keys"
import { CopyButton } from "@/components/copy-button"
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
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { formatDateTime } from "@/lib/time"

import { CodeSnippet } from "./code-snippet"
import {
  accountNameError,
  defaultExpiry,
  descriptionError,
  expiresAtFor,
  expiryOptions,
  keyNameError,
  type ExpiryValue,
} from "./key-status"
import {
  instanceQuery,
  problemText,
  useCreateKey,
  useCreateServiceAccount,
  type ServiceAccount,
} from "./queries"

type Step = "account" | "key" | "reveal"

type FlowProps = {
  startAccount: ServiceAccount | null
  nameInput: RefObject<HTMLInputElement | null>
  keyNameInput: RefObject<HTMLInputElement | null>
  onBusyChange: (busy: boolean) => void
  onClose: () => void
}

type Props = {
  open: boolean
  onOpenChange: (open: boolean) => void
  account: ServiceAccount | null
  finalFocus: () => HTMLElement | null
}

const expiryItems = expiryOptions.map(({ value, label }) => ({ value, label }))

const titleClass = "pr-8 leading-snug wrap-anywhere"

function FieldError({ id, text }: { id: string; text: string | null }) {
  if (text === null) return null
  return (
    <p id={id} role="alert" className="text-sm text-destructive">
      {text}
    </p>
  )
}

function Spinner() {
  return (
    <LoaderCircleIcon
      aria-hidden="true"
      className="size-3.5 motion-safe:animate-spin"
    />
  )
}

function NewKeyFlow({
  startAccount,
  nameInput,
  keyNameInput,
  onBusyChange,
  onClose,
}: FlowProps) {
  const queryClient = useQueryClient()
  const instance = useQuery(instanceQuery())
  const [step, setStep] = useState<Step>(
    startAccount === null ? "account" : "key"
  )
  const [account, setAccount] = useState(startAccount)
  const [accountName, setAccountName] = useState("")
  const [description, setDescription] = useState("")
  const [keyName, setKeyName] = useState("")
  const [expiry, setExpiry] = useState<ExpiryValue>(defaultExpiry)
  const [submitted, setSubmitted] = useState(false)
  const [problem, setProblem] = useState<string | null>(null)
  const [token, setToken] = useState<string | null>(null)
  const descriptionInput = useRef<HTMLInputElement>(null)
  const copyButton = useRef<HTMLButtonElement>(null)
  const expiresTrigger = useRef<HTMLButtonElement>(null)
  const tokenInput = useRef<HTMLInputElement>(null)
  const createAccount = useCreateServiceAccount()
  const createKey = useCreateKey(account?.id ?? "")
  const ids = useId()
  const pending = createAccount.isPending || createKey.isPending

  useEffect(() => {
    onBusyChange(pending)
  }, [pending, onBusyChange])

  useEffect(() => () => onBusyChange(false), [onBusyChange])

  useEffect(() => {
    if (step === "key") keyNameInput.current?.focus()
    if (step === "reveal") copyButton.current?.focus()
  }, [step, keyNameInput])

  useEffect(() => {
    if (step === "reveal" && tokenInput.current !== null) {
      tokenInput.current.value = token ?? ""
    }
  }, [step, token])

  const accountNameProblem = accountNameError(accountName)
  const descriptionProblem = descriptionError(description)
  const keyNameProblem = keyNameError(keyName)
  const showAccountNameError =
    accountNameProblem !== null && (submitted || accountName.length > 0)
  const showDescriptionError = descriptionProblem !== null
  const showKeyNameError =
    keyNameProblem !== null && (submitted || keyName.length > 0)
  const expiresAt = expiresAtFor(expiry)

  function submitAccount(event: FormEvent) {
    event.preventDefault()
    setSubmitted(true)
    if (pending) return
    if (accountNameProblem !== null) {
      nameInput.current?.focus()
      return
    }
    if (descriptionProblem !== null) {
      descriptionInput.current?.focus()
      return
    }
    setProblem(null)
    const trimmed = description.trim()
    createAccount.mutate(
      { name: accountName, description: trimmed === "" ? undefined : trimmed },
      {
        onSuccess: (created) => {
          setAccount(created)
          setSubmitted(false)
          setProblem(null)
          setStep("key")
        },
        onError: (error) =>
          setProblem(problemText(error, { accountName, what: "account" })),
      }
    )
  }

  function submitKey(event: FormEvent) {
    event.preventDefault()
    setSubmitted(true)
    if (pending || account === null) return
    if (keyNameProblem !== null) {
      keyNameInput.current?.focus()
      return
    }
    setProblem(null)
    createKey.mutate(
      { name: keyName.trim(), expires_at: expiresAtFor(expiry) },
      {
        onSuccess: (response) => {
          setToken(response.token)
          setSubmitted(false)
          setStep("reveal")
        },
        onError: (error) => {
          setProblem(problemText(error, { what: "key" }))
          if (error instanceof ApiError && error.status === 404) {
            void queryClient.invalidateQueries({
              queryKey: serviceAccountKeys.list(),
            })
          }
        },
      }
    )
  }

  function selectToken() {
    tokenInput.current?.focus()
    tokenInput.current?.select()
  }

  const nameErrorId = `${ids}-name-error`
  const nameHintId = `${ids}-name-hint`
  const descriptionErrorId = `${ids}-description-error`
  const keyNameErrorId = `${ids}-key-name-error`
  const expiresId = `${ids}-expires`

  if (step === "account") {
    return (
      <form className="grid gap-4" onSubmit={submitAccount} noValidate>
        <DialogHeader>
          <DialogTitle className={titleClass}>New service account</DialogTitle>
          <DialogDescription>
            A service account is the identity a CLI, an MCP client or a script
            signs in as.
          </DialogDescription>
        </DialogHeader>
        <div className="grid gap-2">
          <Label htmlFor={`${ids}-name`}>Name</Label>
          <Input
            id={`${ids}-name`}
            ref={nameInput}
            value={accountName}
            autoComplete="off"
            spellCheck={false}
            readOnly={pending}
            aria-invalid={showAccountNameError}
            aria-describedby={
              showAccountNameError ? `${nameHintId} ${nameErrorId}` : nameHintId
            }
            onChange={(event) => setAccountName(event.target.value)}
          />
          <p id={nameHintId} className="text-sm text-muted-foreground">
            Lowercase letters, digits and dashes, for example{" "}
            <span className="whitespace-nowrap">ci-bot</span> or{" "}
            <span className="whitespace-nowrap">ishan-laptop</span>.
          </p>
          <FieldError
            id={nameErrorId}
            text={showAccountNameError ? accountNameProblem : null}
          />
        </div>
        <div className="grid gap-2">
          <Label htmlFor={`${ids}-description`}>Description (optional)</Label>
          <Input
            id={`${ids}-description`}
            ref={descriptionInput}
            value={description}
            autoComplete="off"
            readOnly={pending}
            aria-invalid={showDescriptionError}
            aria-describedby={
              showDescriptionError ? descriptionErrorId : undefined
            }
            onChange={(event) => setDescription(event.target.value)}
          />
          <FieldError
            id={descriptionErrorId}
            text={showDescriptionError ? descriptionProblem : null}
          />
        </div>
        {problem !== null ? (
          <Alert variant="destructive">
            <AlertDescription>{problem}</AlertDescription>
          </Alert>
        ) : null}
        <DialogFooter>
          <Button
            type="submit"
            focusableWhenDisabled
            disabled={pending}
            aria-busy={pending}
          >
            {pending ? <Spinner /> : null}
            Create service account
          </Button>
        </DialogFooter>
      </form>
    )
  }

  if (account === null) return null

  if (step === "key") {
    return (
      <form className="grid gap-4" onSubmit={submitKey} noValidate>
        <DialogHeader>
          <DialogTitle className={titleClass}>
            Create a key for {account.name}
          </DialogTitle>
          <DialogDescription>
            Name the key after where it will be used, so you know what breaks
            when you revoke it.
          </DialogDescription>
        </DialogHeader>
        <div className="grid gap-2">
          <Label htmlFor={`${ids}-key-name`}>Key name</Label>
          <Input
            id={`${ids}-key-name`}
            ref={keyNameInput}
            value={keyName}
            placeholder="laptop"
            autoComplete="off"
            spellCheck={false}
            readOnly={pending}
            aria-invalid={showKeyNameError}
            aria-describedby={showKeyNameError ? keyNameErrorId : undefined}
            onChange={(event) => setKeyName(event.target.value)}
          />
          <FieldError
            id={keyNameErrorId}
            text={showKeyNameError ? keyNameProblem : null}
          />
        </div>
        <div className="grid gap-2">
          <Label htmlFor={expiresId}>Expires</Label>
          <Select
            items={expiryItems}
            value={expiry}
            disabled={pending}
            onValueChange={(next) => {
              if (next !== null) setExpiry(next)
            }}
          >
            <SelectTrigger
              id={expiresId}
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
          <p className="text-sm text-muted-foreground">
            {expiresAt === null
              ? "Never expires. Revoke it when you no longer need it."
              : `Stops working on ${formatDateTime(expiresAt).slice(0, 10)}.`}
          </p>
        </div>
        {problem !== null ? (
          <Alert variant="destructive">
            <AlertDescription>{problem}</AlertDescription>
          </Alert>
        ) : null}
        <DialogFooter>
          <Button
            type="submit"
            focusableWhenDisabled
            disabled={pending}
            aria-busy={pending}
          >
            {pending ? <Spinner /> : null}
            Create key
          </Button>
        </DialogFooter>
      </form>
    )
  }

  return (
    <div className="grid gap-4">
      <DialogHeader>
        <DialogTitle className={titleClass}>
          Copy the key for {account.name}
        </DialogTitle>
        <DialogDescription>
          This is the only time the key is shown. Copy it now and keep it in a
          password manager, or paste it into observe login.
        </DialogDescription>
      </DialogHeader>
      <div className="grid gap-2">
        <Label htmlFor={`${ids}-token`}>Key</Label>
        <div className="flex gap-2">
          <Input
            id={`${ids}-token`}
            ref={tokenInput}
            readOnly
            autoComplete="off"
            spellCheck={false}
            className="font-mono"
            onFocus={(event) => event.currentTarget.select()}
          />
          <CopyButton
            ref={copyButton}
            text={token ?? ""}
            label="Copy key"
            onCopyFailed={selectToken}
          >
            Copy key
          </CopyButton>
        </div>
      </div>
      {instance.data ? (
        <div className="grid gap-2">
          <p className="text-sm text-muted-foreground">
            Next, sign in the CLI:
          </p>
          <CodeSnippet
            code={`observe login --server ${instance.data.public_url}`}
            label="observe CLI command"
            wrap
          />
        </div>
      ) : null}
      <DialogFooter>
        <Button variant="outline" onClick={onClose}>
          Done
        </Button>
      </DialogFooter>
    </div>
  )
}

export function NewKeyDialog({
  open,
  onOpenChange,
  account,
  finalFocus,
}: Props) {
  const [openCount, setOpenCount] = useState(0)
  const [wasOpen, setWasOpen] = useState(open)
  const [busy, setBusy] = useState(false)
  const nameInput = useRef<HTMLInputElement>(null)
  const keyNameInput = useRef<HTMLInputElement>(null)

  if (open !== wasOpen) {
    setWasOpen(open)
    if (open) setOpenCount((count) => count + 1)
  }

  function changeOpen(next: boolean) {
    if (!next && busy) return
    onOpenChange(next)
  }

  return (
    <Dialog open={open} onOpenChange={changeOpen} disablePointerDismissal>
      <DialogContent
        className="sm:max-w-md"
        initialFocus={account === null ? nameInput : keyNameInput}
        finalFocus={finalFocus}
      >
        <NewKeyFlow
          key={openCount}
          startAccount={account}
          nameInput={nameInput}
          keyNameInput={keyNameInput}
          onBusyChange={setBusy}
          onClose={() => changeOpen(false)}
        />
      </DialogContent>
    </Dialog>
  )
}
