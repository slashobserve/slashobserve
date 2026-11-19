import { Link } from "@tanstack/react-router"

import { docsUrl } from "@/lib/docs"
import { cn } from "@/lib/utils"

import { HomeSection } from "./home-section"

const tileClass =
  "flex h-full flex-col gap-0.5 rounded-card border bg-card px-3 py-2.5 text-foreground no-underline outline-none hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring narrow:min-h-11"

const places = [
  {
    to: "/dashboards",
    label: "Dashboards",
    text: "See what the team already watches.",
  },
  { to: "/logs", label: "Logs", text: "Search logs, one source at a time." },
  {
    to: "/traces",
    label: "Traces",
    text: "Follow one request across services.",
  },
  {
    to: "/observations",
    label: "Observations",
    text: "Notes and findings the team keeps while it works a question.",
  },
] as const

function Tile({ label, text }: { label: string; text: string }) {
  return (
    <>
      <span className="text-[13px] font-medium">{label}</span>
      <span className="text-xs text-muted-foreground">{text}</span>
    </>
  )
}

export function StartHere() {
  return (
    <HomeSection title="Start here">
      <ul className="grid grid-cols-2 gap-2 narrow:grid-cols-1">
        {places.map(({ to, label, text }) => (
          <li key={to}>
            <Link to={to} className={cn(tileClass)}>
              <Tile label={label} text={text} />
            </Link>
          </li>
        ))}
        <li className="col-span-full">
          <a
            href={docsUrl}
            target="_blank"
            rel="noreferrer"
            className={tileClass}
          >
            <Tile
              label="Docs"
              text="What each page does and how the agent works."
            />
          </a>
        </li>
      </ul>
    </HomeSection>
  )
}
