import { ChevronDownIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"

import type { ListSearch } from "./list-filters"

type Props = {
  value: ListSearch["sort"]
  onChange: (value: ListSearch["sort"]) => void
}

const grouped = "folders"

function sortText(value: ListSearch["sort"]): string {
  return value === "views" ? "most viewed" : "by folder"
}

export function SortMenu({ value, onChange }: Props) {
  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={
          <Button
            variant="outline"
            className="max-w-full min-w-0 font-normal"
          />
        }
      >
        <span className="truncate">{`Sort: ${sortText(value)}`}</span>
        <ChevronDownIcon className="size-3" aria-hidden="true" />
      </DropdownMenuTrigger>
      <DropdownMenuContent className="w-max min-w-(--anchor-width)">
        <DropdownMenuRadioGroup
          value={value ?? grouped}
          onValueChange={(next) =>
            onChange(next === "views" ? "views" : undefined)
          }
        >
          <DropdownMenuRadioItem closeOnClick value={grouped}>
            By folder
          </DropdownMenuRadioItem>
          <DropdownMenuRadioItem closeOnClick value="views">
            Most viewed
          </DropdownMenuRadioItem>
        </DropdownMenuRadioGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
