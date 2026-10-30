import type { CSSProperties, ReactNode } from "react"

import type { CanvasElement, CanvasKind, ServerKind } from "./canvas-model"

const artSize = 100
const defaultStroke = "var(--border)"
const silhouetteStroke = "var(--foreground)"
const defaultLight = "var(--threshold-ok)"

const cloudTemplate =
  "M 0.25 0.88 C 0.10 0.88 0.02 0.72 0.06 0.56 C 0.09 0.42 0.20 0.36 0.30 0.40 C 0.31 0.18 0.46 0.06 0.60 0.12 C 0.72 0.04 0.88 0.14 0.88 0.34 C 0.98 0.38 1.00 0.58 0.96 0.70 C 0.92 0.84 0.84 0.88 0.76 0.88 Z"

const bladeAngles = [0, 120, 240]

export const shapeKinds: readonly CanvasKind[] = [
  "rectangle",
  "ellipse",
  "triangle",
  "cloud",
  "parallelogram",
  "server",
  "windTurbine",
  "droneTop",
  "droneFront",
  "droneSide",
]

type Look = {
  fill: string
  stroke: string
  strokeWidth: number
  light: string
}

function scaledPath(template: string, width: number, height: number): string {
  let index = 0
  return template.replace(/-?\d*\.?\d+/g, (number) => {
    const extent = index++ % 2 === 0 ? width : height
    return String(Math.round(Number(number) * extent * 100) / 100)
  })
}

function geometry(element: CanvasElement, look: Look): ReactNode {
  const { width, height } = element.box
  const style: CSSProperties = {
    fill: look.fill,
    stroke: look.stroke,
    strokeWidth: look.strokeWidth,
    strokeLinejoin: "round",
  }
  switch (element.kind) {
    case "ellipse":
      return (
        <ellipse
          cx={width / 2}
          cy={height / 2}
          rx={width / 2}
          ry={height / 2}
          style={style}
        />
      )
    case "triangle":
      return (
        <polygon
          points={`${width / 2},0 ${width},${height} 0,${height}`}
          style={style}
        />
      )
    case "parallelogram": {
      const lean = width * 0.2
      return (
        <polygon
          points={`${lean},0 ${width},0 ${width - lean},${height} 0,${height}`}
          style={style}
        />
      )
    }
    case "cloud":
      return <path d={scaledPath(cloudTemplate, width, height)} style={style} />
    default:
      return (
        <rect
          width={width}
          height={height}
          rx={element.radius}
          ry={element.radius}
          style={style}
        />
      )
  }
}

function Line({ d, look }: { d: string; look: Look }) {
  return (
    <path
      d={d}
      style={{
        fill: "none",
        stroke: look.stroke,
        strokeWidth: look.strokeWidth,
        strokeLinecap: "round",
      }}
    />
  )
}

function Light({
  x,
  y,
  radius,
  look,
}: {
  x: number
  y: number
  radius: number
  look: Look
}) {
  return <circle cx={x} cy={y} r={radius} style={{ fill: look.light }} />
}

function Outlined({ look, children }: { look: Look; children: ReactNode }) {
  return (
    <g
      style={{
        fill: look.fill,
        stroke: look.stroke,
        strokeWidth: look.strokeWidth,
        strokeLinejoin: "round",
      }}
    >
      {children}
    </g>
  )
}

function Unit({ y, height, look }: { y: number; height: number; look: Look }) {
  const middle = y + height / 2
  return (
    <>
      <rect x={12} y={y} width={76} height={height} rx={3} />
      <path
        d={`M 22 ${middle} H 52`}
        style={{ fill: "none", strokeLinecap: "round" }}
      />
      <Light x={76} y={middle} radius={3.5} look={look} />
    </>
  )
}

function ServerArt({ variant, look }: { variant: ServerKind; look: Look }) {
  switch (variant) {
    case "stack":
      return (
        <Outlined look={look}>
          <Unit y={10} height={22} look={look} />
          <Unit y={39} height={22} look={look} />
          <Unit y={68} height={22} look={look} />
        </Outlined>
      )
    case "database":
      return (
        <>
          <Outlined look={look}>
            <path d="M 16 22 V 78 A 34 10 0 0 0 84 78 V 22" />
            <ellipse cx={50} cy={22} rx={34} ry={10} />
          </Outlined>
          <Line d="M 16 38 A 34 10 0 0 0 84 38" look={look} />
          <Line d="M 16 58 A 34 10 0 0 0 84 58" look={look} />
          <Light x={50} y={58} radius={3.5} look={look} />
        </>
      )
    case "terminal":
      return (
        <>
          <Outlined look={look}>
            <rect x={8} y={18} width={84} height={64} rx={4} />
          </Outlined>
          <Line d="M 8 34 H 92" look={look} />
          <Line d="M 22 50 L 34 59 L 22 68" look={look} />
          <Line d="M 40 68 H 58" look={look} />
          <Light x={80} y={26} radius={3.5} look={look} />
        </>
      )
    default:
      return (
        <Outlined look={look}>
          <Unit y={30} height={40} look={look} />
        </Outlined>
      )
  }
}

function TurbineArt({ look }: { look: Look }) {
  return (
    <Outlined look={look}>
      <polygon points="47,48 53,48 56,96 44,96" />
      {bladeAngles.map((angle) => (
        <path
          key={angle}
          d="M 48 38 L 49.2 6 Q 50 3 50.8 6 L 52 38 Z"
          transform={`rotate(${angle} 50 40)`}
        />
      ))}
      <circle cx={50} cy={40} r={5} />
    </Outlined>
  )
}

function DroneArt({ kind, look }: { kind: CanvasKind; look: Look }) {
  if (kind === "droneFront") {
    return (
      <>
        <Outlined look={look}>
          <rect x={36} y={46} width={28} height={14} rx={4} />
          <ellipse cx={14} cy={40} rx={11} ry={2.5} />
          <ellipse cx={86} cy={40} rx={11} ry={2.5} />
        </Outlined>
        <Line d="M 36 50 L 14 44 V 40" look={look} />
        <Line d="M 64 50 L 86 44 V 40" look={look} />
        <Line d="M 42 60 L 38 72 H 30" look={look} />
        <Line d="M 58 60 L 62 72 H 70" look={look} />
      </>
    )
  }
  if (kind === "droneSide") {
    return (
      <>
        <Outlined look={look}>
          <rect x={30} y={46} width={40} height={14} rx={5} />
          <ellipse cx={50} cy={36} rx={26} ry={2.5} />
        </Outlined>
        <Line d="M 50 46 V 38" look={look} />
        <Line d="M 70 52 L 88 48" look={look} />
        <Line d="M 38 60 L 34 72 M 62 60 L 66 72 M 28 72 H 72" look={look} />
      </>
    )
  }
  const rotors: [number, number][] = [
    [18, 18],
    [82, 18],
    [18, 82],
    [82, 82],
  ]
  return (
    <>
      {rotors.map(([x, y]) => (
        <Line key={`${x}:${y}`} d={`M 50 50 L ${x} ${y}`} look={look} />
      ))}
      <Outlined look={look}>
        {rotors.map(([x, y]) => (
          <circle key={`${x}:${y}`} cx={x} cy={y} r={11} />
        ))}
        <rect x={40} y={40} width={20} height={20} rx={4} />
        <polygon points="46,38 50,30 54,38" />
      </Outlined>
    </>
  )
}

function Drawn({ element, look }: { element: CanvasElement; look: Look }) {
  const art =
    element.kind === "server" ? (
      <ServerArt variant={element.server} look={look} />
    ) : element.kind === "windTurbine" ? (
      <TurbineArt look={look} />
    ) : (
      <DroneArt kind={element.kind} look={look} />
    )
  return (
    <svg
      aria-hidden="true"
      viewBox={`0 0 ${artSize} ${artSize}`}
      preserveAspectRatio="xMidYMid meet"
      className="size-full overflow-visible"
    >
      <g transform={`rotate(${element.angle} 50 50)`}>{art}</g>
    </svg>
  )
}

export function ElementShape({ element }: { element: CanvasElement }) {
  const drawn =
    element.kind === "server" ||
    element.kind === "windTurbine" ||
    element.kind.startsWith("drone")
  const look: Look = {
    fill: element.fill ?? "none",
    stroke: element.stroke ?? (drawn ? silhouetteStroke : defaultStroke),
    strokeWidth: element.borderWidth,
    light: element.accent ?? defaultLight,
  }
  if (drawn) {
    const unit =
      artSize / Math.max(1, Math.min(element.box.width, element.box.height))
    return (
      <Drawn
        element={element}
        look={{ ...look, strokeWidth: look.strokeWidth * unit }}
      />
    )
  }
  const { width, height } = element.box
  return (
    <svg
      aria-hidden="true"
      width={width}
      height={height}
      viewBox={`0 0 ${width} ${height}`}
      className="absolute inset-0 overflow-visible"
    >
      {geometry(element, look)}
    </svg>
  )
}
