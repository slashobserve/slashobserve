import { useId, useRef, useState, type FormEvent } from "react"
import { LoaderCircleIcon, UserPlusIcon } from "lucide-react"

import { problemText } from "@/api/errors"
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
import { roleLabels, type Role } from "@/lib/roles"

import {
  inviteEmailProblem,
  inviteProblems,
  useSaveInvite,
  type Invite,
} from "./invites"
import { RoleSelect } from "./role-select"

const defaultRole: Role = "viewer"

function SignInLink({ invite }: { invite: Invite }) {
  const id = useId()
  return (
    <div className="flex flex-col gap-1.5">
      <Label htmlFor={id}>Sign-in link</Label>
      <div className="flex gap-1.5">
        <Input
          id={id}
          readOnly
          value={invite.sign_in_url}
          className="font-mono narrow:h-11"
          onFocus={(event) => event.target.select()}
        />
        <CopyButton
          text={invite.sign_in_url}
          label="Copy sign-in link"
          variant="outline"
          className="narrow:h-11"
        >
          Copy
        </CopyButton>
      </div>
    </div>
  )
}

export function InviteButton() {
  const trigger = useRef<HTMLButtonElement>(null)
  const emailId = useId()
  const roleId = useId()
  const save = useSaveInvite()
  const [open, setOpen] = useState(false)
  const [email, setEmail] = useState("")
  const [role, setRole] = useState<Role>(defaultRole)
  const [problem, setProblem] = useState<string | null>(null)
  const [saved, setSaved] = useState<Invite | null>(null)

  function close() {
    setOpen(false)
    setEmail("")
    setRole(defaultRole)
    setProblem(null)
    setSaved(null)
    save.reset()
  }

  function submit(event: FormEvent) {
    event.preventDefault()
    if (save.isPending) return
    const found = inviteEmailProblem(email)
    setProblem(found)
    if (found !== null) return
    save.mutate(
      { email, role },
      {
        onSuccess: (invite) => setSaved(invite),
        onError: (error) => setProblem(problemText(error, inviteProblems)),
      }
    )
  }

  return (
    <>
      <Button ref={trigger} onClick={() => setOpen(true)}>
        <UserPlusIcon aria-hidden="true" />
        Invite people
      </Button>
      <Dialog
        open={open}
        onOpenChange={(next) => {
          if (next) setOpen(true)
          else if (!save.isPending) close()
        }}
      >
        <DialogContent finalFocus={trigger}>
          {saved === null ? (
            <form noValidate onSubmit={submit} className="grid gap-4">
              <DialogHeader>
                <DialogTitle>Invite people</DialogTitle>
                <DialogDescription>
                  No email is sent. Share the sign-in link; the role applies
                  when they first sign in with this email, verified by your
                  identity provider.
                </DialogDescription>
              </DialogHeader>
              <div className="flex flex-col gap-1.5">
                <Label htmlFor={emailId}>Email</Label>
                <Input
                  id={emailId}
                  type="email"
                  autoComplete="off"
                  spellCheck={false}
                  placeholder="name@example.com"
                  className="narrow:h-11"
                  value={email}
                  aria-invalid={problem !== null}
                  onChange={(event) => {
                    setEmail(event.target.value)
                    setProblem(null)
                  }}
                />
              </div>
              <div className="flex flex-col gap-1.5">
                <Label htmlFor={roleId}>Role</Label>
                <RoleSelect
                  id={roleId}
                  className="w-full narrow:h-11"
                  value={role}
                  onValueChange={setRole}
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
                  disabled={save.isPending}
                  onClick={close}
                >
                  Cancel
                </Button>
                <Button type="submit" aria-busy={save.isPending}>
                  {save.isPending ? (
                    <LoaderCircleIcon
                      aria-hidden="true"
                      className="motion-safe:animate-spin"
                    />
                  ) : null}
                  Invite
                </Button>
              </DialogFooter>
            </form>
          ) : (
            <div className="grid gap-4">
              <DialogHeader>
                <DialogTitle className="pr-8 leading-snug wrap-anywhere">
                  {saved.email} is invited as {roleLabels[saved.role]}
                </DialogTitle>
                <DialogDescription>
                  Send them this link. The invite stays under Pending invites
                  until they sign in.
                </DialogDescription>
              </DialogHeader>
              <SignInLink invite={saved} />
              <DialogFooter>
                <Button type="button" onClick={close}>
                  Done
                </Button>
              </DialogFooter>
            </div>
          )}
        </DialogContent>
      </Dialog>
    </>
  )
}
