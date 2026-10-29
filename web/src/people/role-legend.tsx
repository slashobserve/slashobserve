import { roleDescriptions, roleLabels, roles } from "@/lib/roles"

export function RoleLegend() {
  return (
    <dl
      aria-label="Roles"
      className="mb-4 flex flex-wrap gap-x-6 gap-y-2.5 rounded-card border px-3.5 py-3 text-[13px]"
    >
      {roles.map((role) => (
        <div key={role} className="min-w-55 flex-1">
          <dt className="font-medium">{roleLabels[role]}</dt>
          <dd className="text-xs text-muted-foreground">
            {roleDescriptions[role]}
          </dd>
        </div>
      ))}
    </dl>
  )
}
