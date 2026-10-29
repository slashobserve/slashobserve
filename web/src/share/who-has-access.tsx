import { Link, useRouteContext } from "@tanstack/react-router"
import { useQuery } from "@tanstack/react-query"

import { instanceQuery } from "@/api/instance"
import { LoadProblem } from "@/components/load-problem"
import { buttonVariants } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { folderPermissionsQuery, folderQuery } from "@/folders/queries"
import { cn } from "@/lib/utils"
import { loadProblemText } from "@/service-accounts/queries"

import { accessRows, canManageAccess, type AccessRow } from "./access-rows"

type Props = {
  folder: { id: string; title: string; general: boolean }
  canManage: boolean
  headingId: string
}

export function WhoHasAccess({ folder, canManage, headingId }: Props) {
  const { principal } = useRouteContext({ from: "/_authed" })
  const folderDetail = useQuery(folderQuery(folder.id))
  const permissions = useQuery(folderPermissionsQuery(folder.id))
  const instance = useQuery(instanceQuery())

  const failed = folderDetail.isError || permissions.isError
  const rows =
    folderDetail.data !== undefined && permissions.data !== undefined
      ? accessRows(
          folderDetail.data,
          permissions.data,
          principal,
          instance.data?.oidc_provider_name || null
        )
      : null

  return (
    <section
      aria-labelledby={headingId}
      className="flex flex-col gap-2"
      aria-busy={rows === null && !failed}
    >
      <div className="flex items-baseline justify-between gap-2">
        <h3 id={headingId} className="text-[13px] font-semibold">
          Who has access
        </h3>
        <span className="text-xs text-muted-foreground">
          From the folder{" "}
          <Link
            to="/dashboards"
            search={{ folder: folder.id }}
            className="text-foreground underline underline-offset-2"
          >
            {folder.title}
          </Link>
        </span>
      </div>
      {rows !== null ? (
        <ul className="rounded-card border">
          {rows.map((row) => (
            <AccessItem key={row.key} row={row} />
          ))}
        </ul>
      ) : failed ? (
        <LoadProblem
          title="Could not load who has access"
          detail={loadProblemText(folderDetail.error ?? permissions.error)}
          retrying={folderDetail.isFetching || permissions.isFetching}
          onRetry={() => {
            void folderDetail.refetch()
            void permissions.refetch()
          }}
        />
      ) : (
        <Skeleton className="h-24 w-full animate-none motion-safe:animate-pulse" />
      )}
      {canManageAccess(folder, canManage) ? (
        <Link
          to="/folders/$folderId"
          params={{ folderId: folder.id }}
          className={cn(
            buttonVariants({ variant: "outline" }),
            "self-start no-underline"
          )}
        >
          Manage folder access
        </Link>
      ) : null}
    </section>
  )
}

function AccessItem({ row }: { row: AccessRow }) {
  return (
    <li className="flex items-center gap-2.5 border-b px-3 py-2 text-[13px] last:border-b-0">
      <span
        aria-hidden="true"
        className={cn(
          "grid size-6 flex-none place-items-center border text-[11px] font-semibold",
          row.person ? "rounded-full" : "rounded-control"
        )}
      >
        {row.name.slice(0, 1).toUpperCase()}
      </span>
      <span className="flex min-w-0 flex-1 flex-col">
        <span className="font-medium wrap-anywhere">{row.name}</span>
        <span className="text-xs wrap-anywhere text-muted-foreground">
          {row.detail}
        </span>
      </span>
      {row.level === "" ? null : <span className="text-xs">{row.level}</span>}
    </li>
  )
}
