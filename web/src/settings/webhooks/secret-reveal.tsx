import { useEffect, useRef } from "react"

import { CopyButton } from "@/components/copy-button"
import { Button } from "@/components/ui/button"
import { SettingsCard } from "@/settings/settings-card"

type Props = { webhookName: string; secret: string; onDone: () => void }

export function SecretReveal({ webhookName, secret, onDone }: Props) {
  const done = useRef<HTMLButtonElement>(null)
  useEffect(() => done.current?.focus(), [])
  return (
    <SettingsCard
      title={`Signing secret for ${webhookName}`}
      hint="Check the signature of every request with it. It is shown only now and cannot be read again; rotate it to get a new one."
    >
      <div className="flex flex-col gap-3 px-4 py-3">
        <div className="flex items-center gap-2">
          <code
            data-testid="webhook-secret"
            className="min-w-0 flex-1 rounded-control border bg-muted px-2.5 py-1.5 font-mono text-xs break-all select-all"
          >
            {secret}
          </code>
          <CopyButton
            text={secret}
            label="Copy signing secret"
            variant="outline"
            size="icon-sm"
          />
        </div>
        <div>
          <Button ref={done} variant="outline" onClick={onDone}>
            I have copied it
          </Button>
        </div>
      </div>
    </SettingsCard>
  )
}
