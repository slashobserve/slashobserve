import { useQuery } from "@tanstack/react-query"
import { Link } from "@tanstack/react-router"

import { instanceQuery } from "@/api/instance"
import { RelativeTime } from "@/components/relative-time"
import { DataGrid, type DataGridColumn } from "@/components/data-grid/data-grid"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { CodeSnippet } from "@/service-accounts/code-snippet"
import { loadProblemText } from "@/service-accounts/queries"
import {
  SettingsCard,
  SettingsProblem,
  SettingsRow,
} from "@/settings/settings-card"
import { SettingsSection } from "@/settings/settings-section"

import { DisconnectButton } from "./disconnect-button"
import { ModeSelect } from "./mode-select"
import { useAllGrants, useMyGrants } from "./queries"
import {
  agentName,
  connectPrompt,
  modeLabel,
  modeNote,
  type ConnectedAgent,
  type GrantScope,
} from "./rows"
import { claudeOAuthSnippet } from "./snippets"

function grantId(grant: ConnectedAgent): string {
  return grant.id
}

const myHint = "MCP clients you approved. Each acts as you."
const ownHint =
  "Change how an agent saves, or disconnect it. Disconnecting ends it at once."
const connectHint =
  "Run this where the agent lives, then approve it in the browser."
const everyoneHint =
  "Every MCP client anyone approved. Disconnecting one ends it at once."

export function ConnectedAgentsSection() {
  return (
    <SettingsSection title="Connected agents" description={myHint}>
      <MyAgentsCards />
      <p className="text-[13px] text-muted-foreground">
        Observe Agent, your connected agents and the service accounts that take
        runs are on the{" "}
        <Link to="/agents" className="underline underline-offset-4">
          Agents page
        </Link>
        .
      </p>
    </SettingsSection>
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

function MyAgentsCards() {
  const query = useMyGrants()
  const title = "Your connected agents"
  if (query.isError && query.data === undefined) {
    return (
      <SettingsCard title={title} hint={ownHint}>
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
      <SettingsCard title={title} hint={ownHint}>
        <SettingsRow label="Agents">
          <Skeleton className="h-4 w-32" />
        </SettingsRow>
      </SettingsCard>
    )
  }
  const prompt = connectPrompt(grants.length)
  return (
    <>
      <SettingsCard title={title} hint={ownHint}>
        {prompt === "none" ? (
          <p className="border-t px-4 py-3 text-[13px] text-muted-foreground">
            No agents connected.
          </p>
        ) : (
          <>
            <GrantsTable grants={grants} scope="mine" label={title} />
            <MoreButton query={query} />
            <p className="border-t px-4 py-3 text-xs text-muted-foreground">
              {modeNote()}
            </p>
          </>
        )}
      </SettingsCard>
      <SettingsCard
        title={prompt === "another" ? "Connect another" : "Connect an agent"}
        hint={connectHint}
      >
        <div className="border-t p-4">
          <ConnectSnippet />
        </div>
      </SettingsCard>
    </>
  )
}

export function EveryonesAgentsCard() {
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
        <p className="border-t px-4 py-3 text-[13px] text-muted-foreground">
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
    <div className="border-t px-4 py-3">
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

function grantColumns(scope: GrantScope): DataGridColumn<ConnectedAgent>[] {
  return [
    {
      id: "agent",
      header: "Agent",
      minWidth: 180,
      flex: 1.5,
      autoHeight: true,
      cell: (grant) => (
        <div className="py-2">
          <div className="font-medium wrap-anywhere">
            {agentName(grant.client.name)}
          </div>
          <div className="text-xs wrap-anywhere text-muted-foreground">
            {grant.client.redirect_host}
          </div>
        </div>
      ),
    },
    ...(scope === "all"
      ? [
          {
            id: "person",
            header: "Person",
            minWidth: 200,
            flex: 1.5,
            autoHeight: true,
            cell: (grant: ConnectedAgent) => (
              <span className="py-2 wrap-anywhere text-muted-foreground">
                {`${grant.person.name} (${grant.person.email})`}
              </span>
            ),
          },
        ]
      : []),
    {
      id: "mode",
      header: "Mode",
      minWidth: 150,
      cell: (grant) =>
        scope === "mine" ? (
          <ModeSelect
            grantId={grant.id}
            mode={grant.mode}
            name={agentName(grant.client.name)}
          />
        ) : (
          <span className="text-muted-foreground">{modeLabel(grant.mode)}</span>
        ),
    },
    {
      id: "connected",
      header: "Connected",
      minWidth: 150,
      cell: (grant) => (
        <span className="text-muted-foreground">
          Connected <RelativeTime iso={grant.created_at} />
        </span>
      ),
    },
    {
      id: "lastUsed",
      header: "Last used",
      minWidth: 150,
      cell: (grant) => (
        <span className="text-muted-foreground">
          {grant.last_used_at === null ? (
            "Not used yet"
          ) : (
            <>
              Last used <RelativeTime iso={grant.last_used_at} />
            </>
          )}
        </span>
      ),
    },
    {
      id: "actions",
      header: "Actions",
      action: true,
      align: "right",
      width: 140,
      cell: (grant) => <DisconnectButton grant={grant} scope={scope} />,
    },
  ]
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
    <DataGrid
      label={label}
      rows={grants}
      columns={grantColumns(scope)}
      rowId={grantId}
      bare
      className="border-t"
    />
  )
}
