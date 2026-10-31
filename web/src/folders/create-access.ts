import { useQuery } from "@tanstack/react-query"

import { foldersQuery, type Folder } from "./queries"

export function canCreateDashboards(folders: readonly Folder[]): boolean {
  return folders.some((folder) => folder.permissions.create)
}

export function useCanCreateDashboards(): boolean | undefined {
  return useQuery({ ...foldersQuery(), select: canCreateDashboards }).data
}
