export function neighbourInviteId(
  ids: readonly string[],
  removedId: string
): string | null {
  const index = ids.indexOf(removedId)
  if (index === -1) return null
  return ids[index + 1] ?? ids[index - 1] ?? null
}

export function withdrawButtonFor(inviteId: string | null): HTMLElement | null {
  if (inviteId === null) return null
  const buttons = document.querySelectorAll<HTMLElement>("[data-withdraw]")
  return (
    [...buttons].find((button) => button.dataset.withdraw === inviteId) ?? null
  )
}
