import { useMemo } from "react"
import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { ExternalLinkIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import { DatasourceListProvider } from "@/datasources/queries"
import { PanelBody } from "@/panels/panel-body"
import { createSharedCursor } from "@/panels/shared-cursor"

import type { PanelPayload } from "./payload"

const headerHeight = 44

type Props = {
  payload: PanelPayload
  width: number
  height: number
  onOpen: (url: string) => void
}

const queryClient = new QueryClient({
  defaultOptions: { queries: { enabled: false, retry: false } },
})

export function PanelApp({ payload, width, height, onOpen }: Props) {
  const cursor = useMemo(() => createSharedCursor(), [])
  const bodyHeight = Math.max(0, height - headerHeight)
  return (
    <QueryClientProvider client={queryClient}>
      <DatasourceListProvider value={payload.datasources}>
        <section
          aria-label={payload.title}
          className="flex flex-col text-foreground"
          style={{ width, height }}
        >
          <header
            className="flex items-center justify-between gap-3 px-1"
            style={{ height: headerHeight }}
          >
            <div className="min-w-0">
              <h1 className="truncate text-sm font-medium">{payload.title}</h1>
              <p className="truncate text-xs text-muted-foreground">
                {payload.from} to {payload.to}
              </p>
            </div>
            <Button
              variant="outline"
              size="sm"
              onClick={() => onOpen(payload.open_url)}
            >
              Open in SlashObserve
              <ExternalLinkIcon aria-hidden />
            </Button>
          </header>
          <div style={{ width, height: bodyHeight }}>
            <PanelBody
              panel={payload.panel}
              data={payload.data}
              width={width}
              height={bodyHeight}
              stateScope="mcp-panel"
              cursor={cursor}
              onTimeRangeSelect={() => undefined}
              vars={payload.variables}
              timezone={payload.timezone}
              from={payload.from}
              to={payload.to}
            />
          </div>
        </section>
      </DatasourceListProvider>
    </QueryClientProvider>
  )
}
