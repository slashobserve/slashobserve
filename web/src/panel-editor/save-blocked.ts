export function saveBlockedText({
  deleted,
  jsonBlocked,
  runBlocked,
}: {
  deleted: boolean
  jsonBlocked: boolean
  runBlocked: boolean
}): string | null {
  if (deleted)
    return "This dashboard was deleted, so this draft cannot be saved."
  if (jsonBlocked) return "Fix the JSON to save."
  if (runBlocked) return "Fix the query to save."
  return null
}
