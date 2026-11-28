import { useQuery, useSuspenseQuery } from "@tanstack/react-query"

import { loadProblemText } from "@/api/errors"
import { Wordmark } from "@/components/brand/wordmark"
import { LoadProblem } from "@/components/load-problem"
import { useDocumentTitle } from "@/lib/document-title"

import { RcaDocument, type UseSnapshot, type RcaView } from "./rca-document"
import {
  publicRcaQuery,
  publicRcaSnapshotQuery,
  type PublicRca,
} from "./queries"

function viewOf(rca: PublicRca): RcaView {
  const {
    published_by: publishedBy,
    superseded_by: supersededBy,
    ...rest
  } = rca
  return { ...rest, publishedBy, supersededBy }
}

export function PublicRcaPage({ token }: { token: string }) {
  const rca = useSuspenseQuery(publicRcaQuery(token)).data
  useDocumentTitle(`${rca.title} - SlashObserve`)
  const useSnapshot: UseSnapshot = (snapshotId) =>
    useQuery(publicRcaSnapshotQuery(token, snapshotId))
  return (
    <div className="flex min-h-svh flex-col bg-background text-foreground">
      <header className="pt-3.5 pr-4 pb-2.5 pl-4">
        <Wordmark />
      </header>
      <main className="mx-auto flex w-full max-w-[720px] flex-col px-4 pt-8 pb-16">
        <RcaDocument rca={viewOf(rca)} useSnapshot={useSnapshot} />
      </main>
    </div>
  )
}

export function PublicRcaLoadError({
  error,
  retrying,
  onRetry,
}: {
  error: unknown
  retrying: boolean
  onRetry: () => void
}) {
  return (
    <main className="mx-auto w-full max-w-lg p-6">
      <LoadProblem
        title="Could not load this RCA"
        detail={loadProblemText(error)}
        retrying={retrying}
        onRetry={onRetry}
      />
    </main>
  )
}
