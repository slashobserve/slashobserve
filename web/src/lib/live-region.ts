// A live region only speaks when its text changes, so repeated identical messages alternate a trailing no-break space.
export function liveRegionText(message: string, count: number): string {
  if (message === "") return ""
  return count % 2 === 0 ? message : `${message}\u00a0`
}
