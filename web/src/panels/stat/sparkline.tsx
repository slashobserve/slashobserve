const width = 600
const height = 40
const inset = 2

function scaled(points: readonly [number, number][]) {
  const times = points.map(([time]) => time)
  const values = points.map(([, value]) => value)
  const [t0, t1] = [Math.min(...times), Math.max(...times)]
  const [v0, v1] = [Math.min(...values), Math.max(...values)]
  return points.map(([time, value]) => {
    const x = t1 === t0 ? 0 : ((time - t0) / (t1 - t0)) * width
    const y =
      v1 === v0
        ? height / 2
        : height - inset - ((value - v0) / (v1 - v0)) * (height - 2 * inset)
    return `${x.toFixed(1)},${y.toFixed(1)}`
  })
}

export function Sparkline({
  points,
  stroke,
  strokeOpacity = 1,
  area,
}: {
  points: readonly [number, number][]
  stroke: string
  strokeOpacity?: number
  area: boolean
}) {
  const line = scaled(points)
  return (
    <svg
      aria-hidden="true"
      viewBox={`0 0 ${width} ${height}`}
      preserveAspectRatio="none"
      className="block size-full"
    >
      {area ? (
        <polygon
          points={`0,${height} ${line.join(" ")} ${width},${height}`}
          style={{ fill: stroke, opacity: 0.12 }}
        />
      ) : null}
      <polyline
        points={line.join(" ")}
        fill="none"
        strokeWidth={1.5}
        strokeLinejoin="round"
        vectorEffect="non-scaling-stroke"
        style={{ stroke, strokeOpacity }}
      />
    </svg>
  )
}
