import { useQuery } from "@tanstack/react-query"

import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { foldersQuery } from "@/folders/queries"

type Props = {
  value: string | null
  onChange: (folderId: string | null) => void
  disabled: boolean
  id: string
}

export function FolderSelect({ value, onChange, disabled, id }: Props) {
  const folders = useQuery(foldersQuery())
  const items = (folders.data ?? [])
    .filter((folder) => folder.permissions.edit)
    .toSorted((a, b) => Number(b.personal) - Number(a.personal))
    .map((folder) => ({
      value: folder.id,
      label: folder.personal ? "My Dashboards" : folder.title,
    }))
  return (
    <Select
      items={items}
      value={value}
      disabled={disabled}
      onValueChange={onChange}
    >
      <SelectTrigger id={id} className="w-full">
        <SelectValue placeholder="Default folder" />
      </SelectTrigger>
      <SelectContent alignItemWithTrigger={false} align="start">
        {items.map((item) => (
          <SelectItem key={item.value} value={item.value}>
            {item.label}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  )
}
