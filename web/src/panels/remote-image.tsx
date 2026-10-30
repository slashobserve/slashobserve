import { useState, type CSSProperties } from "react"

import { assetImageUrl } from "@/lib/remote-assets"

export function RemoteImage({
  src,
  alt,
  className,
  style,
}: {
  src: string
  alt: string
  className?: string
  style?: CSSProperties
}) {
  const proxied = assetImageUrl(src)
  const [failed, setFailed] = useState<string | null>(null)
  if (proxied === null || failed === proxied) return <>{alt}</>
  return (
    <img
      src={proxied}
      alt={alt}
      loading="lazy"
      referrerPolicy="no-referrer"
      className={className}
      style={style}
      onError={() => setFailed(proxied)}
    />
  )
}
