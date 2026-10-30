import type { Cell } from "../table-model"
import type { CellSpec } from "./cell-model"
import {
  sparkHeight,
  sparkShape,
  sparkWidth,
  type SparkOptions,
} from "./sparkline-shape"
import { ValueText } from "./value-text"

export function SparklineCell({
  cell,
  spec,
  values,
  options,
}: {
  cell: Cell
  spec: CellSpec
  values: (number | null)[]
  options: SparkOptions
}) {
  const shape = sparkShape(values, options)
  const stroke = cell.color ?? "var(--series-1)"
  return (
    <span className="inline-flex items-center gap-2">
      <svg
        aria-hidden="true"
        width={sparkWidth}
        height={sparkHeight}
        viewBox={`0 0 ${sparkWidth} ${sparkHeight}`}
        className="shrink-0"
      >
        {shape.areas.map((d, index) => (
          <path
            key={`a${index}`}
            d={d}
            style={{ fill: stroke, opacity: options.fill / 100 }}
          />
        ))}
        {shape.bars.map((bar, index) => (
          <rect key={`b${index}`} {...bar} style={{ fill: stroke }} />
        ))}
        {shape.paths.map((d, index) => (
          <path
            key={`p${index}`}
            d={d}
            fill="none"
            strokeLinejoin="round"
            style={{ stroke, strokeWidth: options.lineWidth }}
          />
        ))}
        {shape.dots.map(([x, y], index) => (
          <circle
            key={`d${index}`}
            cx={x}
            cy={y}
            r={options.pointSize / 2}
            style={{ fill: stroke }}
          />
        ))}
      </svg>
      <ValueText cell={cell} spec={spec} />
    </span>
  )
}
