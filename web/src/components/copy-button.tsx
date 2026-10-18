import { useEffect, useRef, useState, type ComponentProps } from "react"
import { CheckIcon, CopyIcon } from "lucide-react"
import { toast } from "sonner"

import { Button } from "@/components/ui/button"

const copiedMs = 2000

type Props = Pick<ComponentProps<typeof Button>, "variant" | "size" | "ref"> & {
  text: string
  label: string
  children?: React.ReactNode
  onCopyFailed?: () => void
}

export function CopyButton({
  text,
  label,
  children,
  variant,
  size,
  ref,
  onCopyFailed,
}: Props) {
  const [copied, setCopied] = useState(false)
  const timer = useRef<number | undefined>(undefined)

  useEffect(() => () => window.clearTimeout(timer.current), [])

  async function copy() {
    try {
      if (!navigator.clipboard?.writeText) throw new Error("unavailable")
      await navigator.clipboard.writeText(text)
    } catch {
      onCopyFailed?.()
      toast.error("Could not copy. Select the text and press Ctrl+C or Cmd+C.")
      return
    }
    setCopied(true)
    window.clearTimeout(timer.current)
    timer.current = window.setTimeout(() => setCopied(false), copiedMs)
  }

  return (
    <>
      <Button
        ref={ref}
        type="button"
        variant={variant}
        size={size}
        aria-label={label}
        onClick={() => void copy()}
      >
        {copied ? (
          <CheckIcon aria-hidden="true" />
        ) : (
          <CopyIcon aria-hidden="true" />
        )}
        {children === undefined ? null : copied ? "Copied" : children}
      </Button>
      <span role="status" aria-live="polite" className="sr-only">
        {copied ? "Copied" : ""}
      </span>
    </>
  )
}
