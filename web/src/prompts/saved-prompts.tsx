import { useId, useRef, useState } from "react"
import { useQuery } from "@tanstack/react-query"
import { useRouteContext } from "@tanstack/react-router"
import { LoaderCircleIcon, LockIcon, UsersIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { useSendExample } from "@/chat/chat-composer"
import { useCoarsePointer } from "@/lib/coarse-pointer"
import { useAgentInfo } from "@/observe/queries"

import { canDriveAgent } from "./access"
import { PromptRowMenu } from "./prompt-row-menu"
import { promptsQuery } from "./queries"
import { scopeOf } from "./scope"

const collapsedCount = 5

export function SavedPrompts() {
  const { principal } = useRouteContext({ from: "/_authed" })
  const agent = useAgentInfo()
  const coarse = useCoarsePointer()
  const headingId = useId()
  const heading = useRef<HTMLHeadingElement>(null)
  const { send, pending } = useSendExample()
  const [runningId, setRunningId] = useState<string | null>(null)
  const [showAll, setShowAll] = useState(false)
  const enabled = canDriveAgent(principal) && agent.data?.available !== false
  const query = useQuery({ ...promptsQuery(), enabled })

  if (!enabled) return null
  const prompts = query.data
  if (prompts !== undefined && prompts.length === 0) return null

  const failed = prompts === undefined && !query.isPending
  const visible = showAll ? prompts : prompts?.slice(0, collapsedCount)

  return (
    <section aria-labelledby={headingId} className="mt-7 w-full max-w-[720px]">
      <div className="flex items-center justify-between px-0.5 pb-2">
        <h2
          id={headingId}
          ref={heading}
          tabIndex={-1}
          className="text-[13px] font-medium text-muted-foreground outline-none"
        >
          Saved prompts
        </h2>
        {coarse ? null : (
          <span className="text-xs text-muted-foreground">
            Type / in the box to pick one
          </span>
        )}
      </div>
      {failed ? (
        <div className="flex items-center gap-3 rounded-card border bg-background px-3.5 py-2.5">
          <p className="text-xs text-muted-foreground">
            Could not load saved prompts.
          </p>
          <Button
            variant="outline"
            size="xs"
            disabled={query.isFetching}
            focusableWhenDisabled
            onClick={() => void query.refetch()}
          >
            Retry
          </Button>
        </div>
      ) : (
        <ul
          aria-busy={query.isPending}
          className="overflow-hidden rounded-card border bg-background"
        >
          {visible === undefined
            ? Array.from({ length: 3 }, (_, index) => (
                <li key={index} className="border-b last:border-b-0">
                  <Skeleton className="h-[52px] rounded-none" />
                </li>
              ))
            : visible.map((prompt) => {
                const scope = scopeOf(prompt, principal.id)
                const ScopeIcon = scope.shared ? UsersIcon : LockIcon
                const running = runningId === prompt.id
                return (
                  <li
                    key={prompt.id}
                    className="flex items-center gap-3 border-b py-[9px] pr-2.5 pl-3.5 last:border-b-0"
                  >
                    <span className="flex min-w-0 flex-1 flex-col">
                      <span className="truncate text-[13px] font-medium">
                        {prompt.name}
                      </span>
                      <span
                        className="truncate text-xs text-muted-foreground"
                        title={prompt.body}
                      >
                        {prompt.body}
                      </span>
                    </span>
                    <span className="flex flex-none items-center gap-[5px] text-xs text-muted-foreground">
                      <ScopeIcon aria-hidden="true" className="size-3" />
                      <span className="narrow:sr-only">{scope.text}</span>
                    </span>
                    <Button
                      variant="outline"
                      size="sm"
                      className="font-normal"
                      aria-label={`Run ${prompt.name}`}
                      aria-busy={running}
                      disabled={pending}
                      focusableWhenDisabled
                      onClick={() => {
                        setRunningId(prompt.id)
                        void send(prompt.body).finally(() => setRunningId(null))
                      }}
                    >
                      {running ? (
                        <LoaderCircleIcon
                          aria-hidden="true"
                          className="motion-safe:animate-spin"
                        />
                      ) : null}
                      Run
                    </Button>
                    {prompt.can_edit ? (
                      <PromptRowMenu prompt={prompt} headingRef={heading} />
                    ) : (
                      <span aria-hidden="true" className="size-7 flex-none" />
                    )}
                  </li>
                )
              })}
        </ul>
      )}
      {prompts !== undefined && prompts.length > collapsedCount ? (
        <Button
          variant="ghost"
          size="sm"
          className="mt-1"
          aria-expanded={showAll}
          onClick={() => setShowAll(!showAll)}
        >
          {showAll ? "Show fewer" : `Show all ${prompts.length}`}
        </Button>
      ) : null}
    </section>
  )
}
