import { ChevronDownIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"

import { listSorts, type ListSearch } from "./list-filters"

type Props = {
  value: ListSearch["sort"]
  onChange: (value: ListSearch["sort"]) => void
}

const grouped = "folders"

const sortLabels = {
  updated: "Last updated",
  name: "Name",
  views: "Most viewed",
}

export function sortText(value: ListSearch["sort"]): string {
  return value === undefined ? "by folder" : sortLabels[value].toLowerCase()
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
            onChange(listSorts.find((known) => known === next))
          }
        >
          <DropdownMenuRadioItem closeOnClick value={grouped}>
            By folder
          </DropdownMenuRadioItem>
          {listSorts.map((sort) => (
            <DropdownMenuRadioItem key={sort} closeOnClick value={sort}>
              {sortLabels[sort]}
            </DropdownMenuRadioItem>
          ))}
        </DropdownMenuRadioGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
