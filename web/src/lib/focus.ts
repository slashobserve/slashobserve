type Focusable = Pick<Element, "closest">

export function focusMovedOutOfMenu<T extends Focusable>(
  active: T | null,
  body: T
): boolean {
  return (
    active !== null &&
    active !== body &&
    active.closest('[role="menu"]') === null
  )
}
