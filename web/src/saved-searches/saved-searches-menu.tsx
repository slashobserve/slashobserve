import { useQuery } from "@tanstack/react-query"
import { useNavigate } from "@tanstack/react-router"

import {
  Combobox,
  ComboboxContent,
  ComboboxEmpty,
  ComboboxInput,
  ComboboxItem,
  ComboboxList,
} from "@/components/ui/combobox"
import { loadProblemText } from "@/service-accounts/queries"

import { folderLabel } from "./folder-label"
import { savedSearchLocation } from "./links"
import { savedSearchesQuery, type SavedSearch } from "./queries"

export function SavedSearchesMenu({ kind }: { kind: SavedSearch["kind"] }) {
  const navigate = useNavigate()
  const query = useQuery(savedSearchesQuery({ kind }))
  const items = query.data?.saved_searches ?? []

  let empty = "No saved searches match."
  if (query.isPending) empty = "Loading..."
  else if (query.isError) empty = loadProblemText(query.error)
  else if (items.length === 0) empty = "No saved searches yet."

  return (
    <Combobox<SavedSearch>
      items={items}
      value={null}
      itemToStringLabel={(saved) => saved.title}
      itemToStringValue={(saved) => saved.id}
      filter={(saved, text) =>
        `${saved.title} ${saved.folder.title}`
          .toLowerCase()
          .includes(text.trim().toLowerCase())
      }
      onValueChange={(saved) => {
        if (saved !== null) void navigate(savedSearchLocation(saved))
      }}
    >
      <ComboboxInput
        aria-label="Saved searches"
        placeholder="Saved searches"
        className="w-44"
      />
      <ComboboxContent className="w-80">
        <ComboboxEmpty>{empty}</ComboboxEmpty>
        <ComboboxList>
          {(saved: SavedSearch) => (
            <ComboboxItem key={saved.id} value={saved}>
              <span className="min-w-0 flex-1 truncate">{saved.title}</span>
              <span className="shrink-0 text-xs text-muted-foreground">
                {folderLabel(saved.folder)}
              </span>
            </ComboboxItem>
          )}
        </ComboboxList>
      </ComboboxContent>
    </Combobox>
  )
}
