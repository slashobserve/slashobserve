import { useQuery } from "@tanstack/react-query"
import { useMatch } from "@tanstack/react-router"
import { ChevronDownIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import {
  dashboardRouteId,
  useDashboardSearch,
  useSetDashboardSearch,
} from "@/dashboards/search"

import { annotationTagsQuery } from "./queries"

export function AnnotationFilter() {
  const match = useMatch({ from: dashboardRouteId, shouldThrow: false })
  if (match === undefined) return null
  return <TagMenu dashboardId={match.params.dashboardId} />
}

function TagMenu({ dashboardId }: { dashboardId: string }) {
  const known = useQuery(annotationTagsQuery(dashboardId)).data ?? []
  const selected = useDashboardSearch().ann ?? []
  const setSearch = useSetDashboardSearch()
  if (known.length === 0 && selected.length === 0) return null

  const choices = [...known, ...selected.filter((tag) => !known.includes(tag))]
  const apply = (tags: string[]) =>
    setSearch(
      (state) => ({ ...state, ann: tags.length === 0 ? undefined : tags }),
      { replace: true }
    )
  const toggle = (tag: string, checked: boolean) =>
    apply(
      checked ? [...selected, tag] : selected.filter((item) => item !== tag)
    )

  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={
          <Button
            variant="outline"
            className="max-w-[24ch] min-w-0 font-normal"
          />
        }
      >
        <span className="truncate">
          {`Annotations: ${selected.length === 0 ? "All" : selected.join(", ")}`}
        </span>
        <ChevronDownIcon className="size-3" aria-hidden="true" />
      </DropdownMenuTrigger>
      <DropdownMenuContent className="w-max max-w-[calc(100vw-32px)] min-w-(--anchor-width) wrap-anywhere">
        <DropdownMenuItem onClick={() => apply([])}>
          All annotations
        </DropdownMenuItem>
        <DropdownMenuSeparator />
        {choices.map((tag) => (
          <DropdownMenuCheckboxItem
            key={tag}
            checked={selected.includes(tag)}
            onCheckedChange={(checked) => toggle(tag, checked)}
          >
            {tag}
          </DropdownMenuCheckboxItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
