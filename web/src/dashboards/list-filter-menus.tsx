import { useQuery } from "@tanstack/react-query"
import { ChevronDownIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { datasourcesQuery } from "@/datasources/queries"
import { usersQuery } from "@/people/queries"

import { madeByText } from "./list-filters"

type MenuProps = {
  value: string | undefined
  onChange: (value: string | undefined) => void
}

const contentClass = "min-w-max max-w-[calc(100vw-32px)]"

function Trigger({ children }: { children: string }) {
  return (
    <DropdownMenuTrigger
      render={<Button variant="outline" className="font-normal" />}
    >
      {children}
      <ChevronDownIcon className="size-3" aria-hidden="true" />
    </DropdownMenuTrigger>
  )
}

export function MadeByMenu({ value, onChange }: MenuProps) {
  const users = useQuery(usersQuery()).data ?? []
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
