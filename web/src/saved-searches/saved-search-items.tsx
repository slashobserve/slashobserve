import { useQuery } from "@tanstack/react-query"
import { useNavigate } from "@tanstack/react-router"

import { DropdownMenuItem } from "@/components/ui/dropdown-menu"
import { loadProblemText } from "@/service-accounts/queries"

import { savedSearchLocation } from "./links"
import { savedSearchesQuery, type SavedSearch } from "./queries"

export function SavedSearchItems({ kind }: { kind: SavedSearch["kind"] }) {
  const navigate = useNavigate()
  const query = useQuery(savedSearchesQuery({ kind }))
  const items = query.data?.saved_searches ?? []
  if (query.isPending) {
    return <DropdownMenuItem disabled>Loading...</DropdownMenuItem>
  }
  if (query.isError) {
    return (
      <DropdownMenuItem disabled>
        {loadProblemText(query.error)}
      </DropdownMenuItem>
    )
  }
  if (items.length === 0) {
    return <DropdownMenuItem disabled>No saved searches yet.</DropdownMenuItem>
  }
  return items.map((saved) => (
    <DropdownMenuItem
      key={saved.id}
      onClick={() => void navigate(savedSearchLocation(saved))}
    >
      <span className="min-w-0 flex-1 truncate">{saved.title}</span>
    </DropdownMenuItem>
  ))
}
