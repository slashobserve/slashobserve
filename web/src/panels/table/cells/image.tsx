import { RemoteImage } from "../../remote-image"

export function ImageCell({
  value,
  alt,
  title,
}: {
  value: unknown
  alt: string
  title?: string
}) {
  if (typeof value !== "string" || value.trim() === "") {
    return <span className="text-muted-foreground">-</span>
  }
  return (
    <span title={title} className="inline-flex items-center">
      <RemoteImage
        src={value.trim()}
        alt={alt}
        className="h-6 max-w-32 object-contain"
      />
    </span>
  )
}
