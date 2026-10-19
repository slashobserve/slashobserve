const maxSlugLength = 60

export function exportFileName(title: string, version: number): string {
  const slug = title
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "")
    .slice(0, maxSlugLength)
    .replace(/-+$/g, "")
  return `${slug === "" ? "dashboard" : slug}-v${version}.json`
}

export function downloadSpec(spec: unknown, title: string, version: number) {
  const blob = new Blob([JSON.stringify(spec, null, 2) + "\n"], {
    type: "application/json",
  })
  const url = URL.createObjectURL(blob)
  const link = document.createElement("a")
  link.href = url
  link.download = exportFileName(title, version)
  document.body.append(link)
  link.click()
  link.remove()
  URL.revokeObjectURL(url)
}
