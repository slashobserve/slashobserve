import { useId } from "react"
import { useQuery } from "@tanstack/react-query"

import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"

import { CodeSnippet } from "./code-snippet"
import { instanceQuery, loadProblemText } from "./queries"

function Code({ children }: { children: string }) {
  return <code className="font-mono text-xs">{children}</code>
}

export function ConnectClients() {
  const headingId = useId()
  const query = useQuery(instanceQuery())

  return (
    <section aria-labelledby={headingId} className="flex flex-col gap-3">
      <div className="flex flex-col gap-1">
        <h2 id={headingId} className="text-lg font-semibold">
          Connect a client
        </h2>
        <p className="text-sm text-muted-foreground">
          Create a key above, then set up a client. The snippets never contain
          the key.
        </p>
      </div>
      {query.isPending ? (
        <Skeleton className="h-24 w-full" />
      ) : query.isError ? (
        <Alert variant="destructive">
          <AlertTitle>Could not load the server address</AlertTitle>
          <AlertDescription>
            <p>{loadProblemText(query.error)}</p>
            <Button
              variant="outline"
              size="sm"
              className="text-foreground"
              disabled={query.isFetching}
              focusableWhenDisabled
              aria-busy={query.isFetching}
              onClick={() => void query.refetch()}
            >
              Retry
            </Button>
          </AlertDescription>
        </Alert>
      ) : (
        <Snippets
          publicUrl={query.data.public_url}
          mcpUrl={query.data.mcp_url}
        />
      )}
    </section>
  )
}

function Snippets({
  publicUrl,
  mcpUrl,
}: {
  publicUrl: string
  mcpUrl: string
}) {
  const claude = JSON.stringify(
    {
      mcpServers: {
        slashobserve: {
          type: "http",
          url: mcpUrl,
          headers: { Authorization: "Bearer ${OBSERVE_TOKEN}" },
        },
      },
    },
    null,
    2
  )
  const opencode = JSON.stringify(
    {
      $schema: "https://opencode.ai/config.json",
      mcp: {
        slashobserve: {
          type: "remote",
          url: mcpUrl,
          enabled: true,
          oauth: false,
          headers: { Authorization: "Bearer {env:OBSERVE_TOKEN}" },
        },
      },
    },
    null,
    2
  )

  return (
    <Tabs defaultValue="cli">
      <TabsList activateOnFocus>
        <TabsTrigger value="cli">observe CLI</TabsTrigger>
        <TabsTrigger value="claude">Claude Code</TabsTrigger>
        <TabsTrigger value="opencode">OpenCode</TabsTrigger>
      </TabsList>
      <TabsContent value="cli" className="flex flex-col gap-3">
        <p className="text-muted-foreground">
          Run this, then paste the key when asked. The CLI checks the key and
          saves it for this server.
        </p>
        <CodeSnippet
          code={`observe login --server ${publicUrl}`}
          label="observe CLI command"
          wrap
        />
      </TabsContent>
      <TabsContent value="claude" className="flex flex-col gap-3">
        <p className="text-muted-foreground">
          Add this to .mcp.json in your project. Claude Code expands{" "}
          <Code>OBSERVE_TOKEN</Code> from the environment, so the key is never
          stored in the file.
        </p>
        <CodeSnippet
          code={claude}
          label="Claude Code configuration"
          wrap={false}
        />
      </TabsContent>
      <TabsContent value="opencode" className="flex flex-col gap-3">
        <p className="text-muted-foreground">
          Add this to opencode.json. OpenCode reads <Code>OBSERVE_TOKEN</Code>{" "}
          from the environment when it starts.
        </p>
        <CodeSnippet
          code={opencode}
          label="OpenCode configuration"
          wrap={false}
        />
      </TabsContent>
    </Tabs>
  )
}
