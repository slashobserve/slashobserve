import { useQuery } from "@tanstack/react-query"
import { useRouteContext } from "@tanstack/react-router"

import { instanceQuery } from "@/api/instance"
import { RelativeTime } from "@/components/relative-time"
import { Cell, HeaderCell, TableCard } from "@/components/table-card"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { roleAtLeast } from "@/lib/roles"
import { CodeSnippet } from "@/service-accounts/code-snippet"
import { loadProblemText } from "@/service-accounts/queries"
import {
  SettingsCard,
  SettingsProblem,
  SettingsRow,
} from "@/settings/settings-card"

import { DisconnectButton } from "./disconnect-button"
import { ModeSelect } from "./mode-select"
import { useAllGrants, useMyGrants } from "./queries"
import {
  agentName,
  connectPrompt,
  modeLabel,
  type ConnectedAgent,
  type GrantScope,
} from "./rows"
import { claudeOAuthSnippet } from "./snippets"

const myHint = "MCP clients you approved. Each acts as you."
const everyoneHint =
  "Every MCP client anyone approved. Disconnecting one ends it at once."

export function ConnectedAgentsSection() {
  const { principal } = useRouteContext({ from: "/_authed" })
  return (
    <>
      <MyAgentsCard />
      {roleAtLeast(principal.role, "admin") ? <EveryonesAgentsCard /> : null}
    </>
  )
}

function ConnectSnippet() {
  const instance = useQuery(instanceQuery())
  if (instance.data === undefined) return null
  return (
    <CodeSnippet
      code={claudeOAuthSnippet(instance.data.mcp_url)}
      label="Claude Code connect command"
    />
  )
}

function MyAgentsCard() {
  const query = useMyGrants()
  const title = "Connected agents"
  if (query.isError && query.data === undefined) {
    return (
      <SettingsCard title={title} hint={myHint}>
        <SettingsProblem
          title="Could not load connected agents"
          detail={loadProblemText(query.error)}
          retrying={query.isFetching}
          onRetry={() => void query.refetch()}
        />
      </SettingsCard>
    )
  }
  const grants = query.data
  if (grants === undefined) {
    return (
      <SettingsCard title={title} hint={myHint}>
        <SettingsRow label="Agents">
          <Skeleton className="h-4 w-32" />
        </SettingsRow>
      </SettingsCard>
    )
  }
  const prompt = connectPrompt(grants.length)
  return (
    <SettingsCard title={title} hint={myHint}>
      {prompt === "none" ? (
        <p className="px-4 pt-4 text-[13px] text-muted-foreground">
          No agents connected.
        </p>
      ) : (
        <GrantsTable grants={grants} scope="mine" label="Connected agents" />
      )}
      <MoreButton query={query} />
      <div className="flex flex-col gap-2 p-4">
        {prompt === "another" ? (
          <h3 className="text-[13px] font-medium">Connect another</h3>
        ) : null}
        <ConnectSnippet />
      </div>
    </SettingsCard>
  )
}

function EveryonesAgentsCard() {
  const query = useAllGrants()
  const title = "Everyone's connected agents"
  if (query.isError && query.data === undefined) {
    return (
      <SettingsCard title={title} hint={everyoneHint}>
        <SettingsProblem
          title="Could not load everyone's connected agents"
          detail={loadProblemText(query.error)}
          retrying={query.isFetching}
          onRetry={() => void query.refetch()}
        />
      </SettingsCard>
    )
  }
  const grants = query.data
  return (
    <SettingsCard title={title} hint={everyoneHint}>
      {grants === undefined ? (
        <SettingsRow label="Agents">
          <Skeleton className="h-4 w-32" />
        </SettingsRow>
      ) : grants.length === 0 ? (
        <p className="p-4 text-[13px] text-muted-foreground">
          No agents connected.
        </p>
      ) : (
        <GrantsTable grants={grants} scope="all" label={title} />
      )}
      <MoreButton query={query} />
    </SettingsCard>
  )
}

function MoreButton({
  query,
}: {
  query: Pick<
    ReturnType<typeof useMyGrants>,
    "hasNextPage" | "isFetchingNextPage" | "fetchNextPage"
  >
}) {
  if (!query.hasNextPage) return null
  return (
    <div className="px-4 pb-4">
      <Button
        variant="outline"
        size="sm"
        disabled={query.isFetchingNextPage}
        onClick={() => void query.fetchNextPage()}
      >
        Show more
      </Button>
    </div>
  )
}

function GrantsTable({
  grants,
  scope,
  label,
}: {
  grants: readonly ConnectedAgent[]
  scope: GrantScope
  label: string
}) {
  return (
    <TableCard
      label={label}
      minWidth={scope === "all" ? 720 : 600}
      stackOnNarrow
      className="rounded-none border-0"
    >
      <thead>
        <tr>
          <HeaderCell>Agent</HeaderCell>
          {scope === "all" ? <HeaderCell>Person</HeaderCell> : null}
          <HeaderCell>Mode</HeaderCell>
          <HeaderCell>Connected</HeaderCell>
          <HeaderCell>Last used</HeaderCell>
          <HeaderCell>
            <span className="sr-only">Actions</span>
          </HeaderCell>
        </tr>
      </thead>
      <tbody>
        {grants.map((grant) => (
          <tr key={grant.id}>
            <Cell>
              <div className="font-medium wrap-anywhere">
                {agentName(grant.client.name)}
              </div>
              <div className="text-xs wrap-anywhere text-muted-foreground">
                {grant.client.redirect_host}
              </div>
            </Cell>
            {scope === "all" ? (
              <Cell className="wrap-anywhere text-muted-foreground">
                {`${grant.person.name} (${grant.person.email})`}
              </Cell>
            ) : null}
            <Cell
              className={scope === "all" ? "text-muted-foreground" : undefined}
            >
              {scope === "mine" ? (
                <ModeSelect
                  grantId={grant.id}
                  mode={grant.mode}
                  name={agentName(grant.client.name)}
                />
              ) : (
                modeLabel(grant.mode)
              )}
            </Cell>
            <Cell className="text-muted-foreground">
              Connected <RelativeTime iso={grant.created_at} />
            </Cell>
            <Cell className="text-muted-foreground">
              {grant.last_used_at === null ? (
                "Not used yet"
              ) : (
                <>
                  Last used <RelativeTime iso={grant.last_used_at} />
                </>
              )}
            </Cell>
            <Cell data-cell="action" className="text-right">
              <DisconnectButton grant={grant} scope={scope} />
            </Cell>
          </tr>
        ))}
      </tbody>
    </TableCard>
  )
}
