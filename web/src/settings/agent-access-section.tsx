import { Link } from "@tanstack/react-router"
import { ArrowRightIcon } from "lucide-react"

import { buttonVariants } from "@/components/ui/button"
import { EveryonesAgentsCard } from "@/connected-agents/connected-agents-card"
import { cn } from "@/lib/utils"

import { SettingsCard } from "./settings-card"
import { SettingsSection } from "./settings-section"

export function AgentAccessSection() {
  return (
    <SettingsSection
      title="Agent access"
      description="Who and what can act on this workspace from outside the browser."
    >
      <EveryonesAgentsCard />
      <SettingsCard
        title="API keys"
        hint="Keys let Claude Code, OpenCode and the observe CLI use this server over MCP and the API."
        action={
          <Link
            to="/keys"
            className={cn(
              buttonVariants({ variant: "outline", size: "sm" }),
              "narrow:h-11"
            )}
          >
            Open API keys
            <ArrowRightIcon aria-hidden="true" />
          </Link>
        }
      />
    </SettingsSection>
  )
}
