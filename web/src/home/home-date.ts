export function homeDate(now: Date, locale?: string): string {
  const weekday = new Intl.DateTimeFormat(locale, { weekday: "long" })
  const dayAndMonth = new Intl.DateTimeFormat(locale, {
    day: "numeric",
    month: "long",
  })
  return `${weekday.format(now)}, ${dayAndMonth.format(now)}`
}
