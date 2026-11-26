import type { ReactNode } from "react"
import { Link } from "@tanstack/react-router"

import { PageHeader } from "@/components/page-header"
import { buttonVariants } from "@/components/ui/button"
import { cn } from "@/lib/utils"

import { exploreText, type ExploreKind } from "./sources"

export function UnknownSource({
  kind,
  uid,
  children,
}: {
  kind: ExploreKind
  uid: string
  children?: ReactNode
}) {
  const { page, path } = exploreText[kind]
  return (
    <>
      <PageHeader
        title={`No ${page.toLowerCase()} datasource named ${uid}`}
        description={`${page} opens only on a datasource of its own kind. Check the address, or choose a source from the ${page} page.`}
      />
      {children}
      <div>
        <Link to={path} className={cn(buttonVariants({ variant: "outline" }))}>
          Open {page}
        </Link>
      </div>
    </>
  )
}
