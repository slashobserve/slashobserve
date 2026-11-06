import { roleDescriptions, roleLabels, roles } from "@/lib/roles"

export function RoleLegend() {
  return (
    <>
      <dl
        aria-label="Roles"
        className="mb-4 grid grid-cols-4 gap-4 rounded-card border px-3.5 py-3 text-[13px] narrow:grid-cols-2"
      >
        {roles.map((role) => (
          <div key={role} className="min-w-0">
            <dt className="font-medium">{roleLabels[role]}</dt>
            <dd className="text-xs text-muted-foreground">
              {roleDescriptions[role]}
            </dd>
          </div>
        ))}
      </dl>
      <p className="mb-4 text-xs text-muted-foreground">
        A restricted folder&apos;s list can give a person more than their role
        on that folder, never less.
      </p>
    </>
  )
}
