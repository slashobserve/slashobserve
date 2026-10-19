import { CopyButton } from "@/components/copy-button"

type Props = { code: string; label: string }

export function CodeSnippet({ code, label }: Props) {
  return (
    <div className="flex items-start gap-2 rounded-[8px] border p-3">
      <pre
        tabIndex={0}
        role="region"
        aria-label={label}
        className="min-w-0 flex-1 font-mono text-xs leading-[1.6] break-all whitespace-pre-wrap outline-none focus-visible:ring-2 focus-visible:ring-ring"
      >
        <code>{code}</code>
      </pre>
      <CopyButton
        text={code}
        label={`Copy ${label}`}
        variant="outline"
        size="sm"
        className="px-2 [&_svg:not([class*='size-'])]:size-[13px]"
      />
    </div>
  )
}
