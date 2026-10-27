import { Link } from "@tanstack/react-router"
import { FileUpIcon } from "lucide-react"

import { buttonVariants } from "@/components/ui/button"
import { cn } from "@/lib/utils"

export function ImportLink() {
  return (
    <Link
      to="/import"
      className={cn(
        buttonVariants({ variant: "ghost", size: "sm" }),
        "h-8 gap-1.5 text-[13px] font-normal text-muted-foreground"
      )}
    >
      <FileUpIcon aria-hidden="true" />
      Import from Grafana
    </Link>
  )
}
