import { useId } from "react"
import { useQuery } from "@tanstack/react-query"

import { instanceQuery } from "@/api/instance"
import { LoadProblem } from "@/components/load-problem"
import { Skeleton } from "@/components/ui/skeleton"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"

import { CodeSnippet } from "./code-snippet"
import { loadProblemText } from "./queries"
import { claudeSnippet, cliSnippet, openCodeSnippet } from "./snippets"

const triggerClass =
  "h-auto flex-none rounded-none border-0 border-b-2 border-transparent px-0 pb-2 text-[13px] font-medium text-muted-foreground -mb-px after:hidden data-active:border-foreground data-active:text-foreground dark:data-active:border-foreground dark:group-data-[variant=line]/tabs-list:data-active:border-foreground"

export function ConnectClients() {
  const headingId = useId()
  const query = useQuery(instanceQuery())

  return (
    <section aria-labelledby={headingId} className="rounded-card border p-4">
      <h2 id={headingId} className="mb-1 text-sm font-semibold">
        Connect a client
      </h2>
      <p className="mb-3 text-xs text-muted-foreground">
        Put the key in OBSERVE_TOKEN, then use one of these. The key itself
        never goes in a file.
      </p>
      {query.isPending ? (
        <Skeleton className="h-24 w-full" />
      ) : query.data === undefined ? (
        <LoadProblem
          title="Could not load the server address"
          detail={loadProblemText(query.error)}
          retrying={query.isFetching}
          onRetry={() => void query.refetch()}
        />
      ) : (
        <Tabs defaultValue="claude">
          <TabsList
            variant="line"
            activateOnFocus
            aria-label="Client"
            className="mb-3 h-auto w-full justify-start gap-[18px] border-b p-0"
          >
            <TabsTrigger value="claude" className={triggerClass}>
              Claude Code
            </TabsTrigger>
            <TabsTrigger value="opencode" className={triggerClass}>
              OpenCode
            </TabsTrigger>
            <TabsTrigger value="cli" className={triggerClass}>
              observe CLI
            </TabsTrigger>
          </TabsList>
          <TabsContent value="claude">
            <CodeSnippet
              code={claudeSnippet(query.data.mcp_url)}
              label="Claude Code setup"
            />
          </TabsContent>
          <TabsContent value="opencode">
            <CodeSnippet
              code={openCodeSnippet(query.data.mcp_url)}
              label="OpenCode configuration"
            />
          </TabsContent>
          <TabsContent value="cli">
            <CodeSnippet
              code={cliSnippet(query.data.public_url)}
              label="observe CLI commands"
            />
          </TabsContent>
        </Tabs>
      )}
    </section>
  )
}
