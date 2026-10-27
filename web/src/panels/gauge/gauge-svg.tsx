import { arcPath, type Gauge } from "./gauge-model"

const trackRadius = 40
const trackWidth = 8
const markerRadius = 46.5
const markerWidth = 3

export function GaugeSvg({ gauge }: { gauge: Gauge }) {
  return (
    <svg
      viewBox="0 0 100 100"
      aria-hidden="true"
      focusable="false"
      className="absolute inset-0 size-full"
    >
      <path
        d={arcPath(0, 1, trackRadius)}
        fill="none"
        strokeWidth={trackWidth}
        style={{ stroke: "var(--border)" }}
      />
      {gauge.markers.map((marker) => (
        <path
          key={`${marker.from}:${marker.color}`}
          d={arcPath(marker.from, marker.to, markerRadius)}
          fill="none"
          strokeWidth={markerWidth}
          style={{ stroke: marker.color }}
        />
      ))}
      <path
        data-gauge-arc=""
        d={arcPath(0, gauge.fraction, trackRadius)}
        fill="none"
        strokeWidth={trackWidth}
        style={{ stroke: gauge.color ?? "var(--series-1)" }}
      />
    </svg>
  )
}
