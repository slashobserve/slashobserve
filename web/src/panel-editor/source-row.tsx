import type { DashboardQuery } from "@/spec"

type Props = {
  refName: string
  query: DashboardQuery
  sourceTitle: string
}

export function sourceSummary(
  source: DashboardQuery["source"],
  title: string
): string {
  return source.withTransforms
    ? `Results of ${title}, after its transforms`
    : `Results of ${title}`
}

export function SourceRow({ refName, query, sourceTitle }: Props) {
  return (
    <div
      role="group"
      aria-label={`Reused panel ${refName}`}
      className="flex flex-col gap-1.5"
    >
      <div className="flex min-h-6 flex-wrap items-center gap-x-2 gap-y-1 text-[13px]">
        <span className="font-medium">Reused panel {refName}</span>
        <span className="text-xs text-muted-foreground">
          {query.hide ? "hidden" : "dashboard"}
        </span>
      </div>
      <p className="rounded-control border border-border px-2 py-1.5 text-[13px] break-words text-muted-foreground">
        {sourceSummary(query.source, sourceTitle)}
      </p>
    </div>
  )
}
