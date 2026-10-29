import { queryOptions, useQuery } from "@tanstack/react-query"
import { ChevronDownIcon } from "lucide-react"

import { api } from "@/api/client"
import { unwrap } from "@/api/errors"
import { dashboardKeys } from "@/api/keys"
import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { datasourcesQuery } from "@/datasources/queries"
import { usersQuery } from "@/people/queries"

import { madeByText, tagsText } from "./list-filters"

type MenuProps = {
  value: string | undefined
  onChange: (value: string | undefined) => void
}

const contentClass =
  "w-max min-w-(--anchor-width) max-w-[calc(100vw-32px)] wrap-anywhere"

function Trigger({ children }: { children: string }) {
  return (
    <DropdownMenuTrigger
      render={
        <Button variant="outline" className="max-w-full min-w-0 font-normal" />
      }
    >
      <span className="truncate">{children}</span>
      <ChevronDownIcon className="size-3" aria-hidden="true" />
    </DropdownMenuTrigger>
  )
}

export function MadeByMenu({ value, onChange }: MenuProps) {
  const users = useQuery({ ...usersQuery(), refetchInterval: false }).data ?? []
  return (
    <DropdownMenu>
      <Trigger>{`Made by: ${madeByText(value, users)}`}</Trigger>
      <DropdownMenuContent className={contentClass}>
        <DropdownMenuRadioGroup
          value={value ?? ""}
          onValueChange={(next) => onChange(next === "" ? undefined : next)}
        >
          <DropdownMenuRadioItem closeOnClick value="">
            Anyone
          </DropdownMenuRadioItem>
          <DropdownMenuRadioItem closeOnClick value="me">
            Me
          </DropdownMenuRadioItem>
          <DropdownMenuRadioItem closeOnClick value="agent">
            Agent
          </DropdownMenuRadioItem>
          <DropdownMenuRadioItem closeOnClick value="mcp">
            MCP clients
          </DropdownMenuRadioItem>
          {users.length > 0 ? (
            <>
              <DropdownMenuSeparator />
              <DropdownMenuLabel>People</DropdownMenuLabel>
              {users.map((user) => (
                <DropdownMenuRadioItem
                  closeOnClick
                  key={user.id}
                  value={`user:${user.id}`}
                >
                  {user.name}
                </DropdownMenuRadioItem>
              ))}
            </>
          ) : null}
        </DropdownMenuRadioGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

export function DatasourceMenu({ value, onChange }: MenuProps) {
  const datasources = useQuery(datasourcesQuery()).data ?? []
  return (
    <DropdownMenu>
      <Trigger>{`Datasource: ${value ?? "any"}`}</Trigger>
      <DropdownMenuContent className={contentClass}>
        <DropdownMenuRadioGroup
          value={value ?? ""}
          onValueChange={(next) => onChange(next === "" ? undefined : next)}
        >
          <DropdownMenuRadioItem closeOnClick value="">
            Any
          </DropdownMenuRadioItem>
          {datasources.map((datasource) => (
            <DropdownMenuRadioItem
              closeOnClick
              key={datasource.uid}
              value={datasource.uid}
            >
              {`${datasource.uid} (${datasource.type})`}
            </DropdownMenuRadioItem>
          ))}
        </DropdownMenuRadioGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

function tagsQuery() {
  return queryOptions({
    queryKey: dashboardKeys.tags(),
    queryFn: ({ signal }) =>
      api.GET("/dashboards/tags", { signal }).then(unwrap),
    staleTime: 60_000,
  })
}

type TagsMenuProps = {
  value: string[] | undefined
  onChange: (tags: string[] | undefined) => void
}

export function TagsMenu({ value, onChange }: TagsMenuProps) {
  const known = useQuery(tagsQuery()).data?.tags ?? []
  const selected = value ?? []
  const missing = selected.filter((tag) => !known.some((t) => t.tag === tag))
  const toggle = (tag: string, checked: boolean) => {
    const next = checked
      ? [...selected, tag]
      : selected.filter((item) => item !== tag)
    onChange(next.length === 0 ? undefined : next)
  }
  return (
    <DropdownMenu>
      <Trigger>{`Tags: ${tagsText(value)}`}</Trigger>
      <DropdownMenuContent className={contentClass}>
        {known.length === 0 && missing.length === 0 ? (
          <DropdownMenuItem disabled>No tags yet</DropdownMenuItem>
        ) : null}
        {known.map(({ tag, dashboards }) => (
          <DropdownMenuCheckboxItem
            key={tag}
            checked={selected.includes(tag)}
            onCheckedChange={(checked) => toggle(tag, checked)}
          >
            {`${tag} (${dashboards})`}
          </DropdownMenuCheckboxItem>
        ))}
        {missing.map((tag) => (
          <DropdownMenuCheckboxItem
            key={tag}
            checked
            onCheckedChange={(checked) => toggle(tag, checked)}
          >
            {tag}
          </DropdownMenuCheckboxItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
