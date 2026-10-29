import { DashboardNotice } from "./not-found"

export function DashboardNoAccess() {
  return (
    <DashboardNotice
      title="No access"
      heading="You don't have access to this dashboard"
      text="It is in a folder you cannot see. Ask someone who manages that folder to add you."
      backTo="/dashboards"
    />
  )
}
