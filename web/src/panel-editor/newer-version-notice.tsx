import { AlertDescription, AlertTitle } from "@/components/ui/alert"

import type { Overlap } from "./changes"

type Props = {
  version: number
  author: string | null
  overlaps: Overlap[]
}

export function newerVersionTitle(version: number, author: string | null) {
  return author
    ? `${author} saved version ${version} while you were editing.`
    : `Version ${version} was saved while you were editing.`
}

function quoted(value: string) {
  return `"${value}"`
}

export function NewerVersionNotice({ version, author, overlaps }: Props) {
  return (
    <>
      <AlertTitle>{newerVersionTitle(version, author)}</AlertTitle>
      <AlertDescription>
        {overlaps.length > 0 ? (
          <div className="flex flex-col gap-1.5">
            <p>
              You both changed the same things. Saving keeps your values for
              these and version {version} for everything else.
            </p>
            <ul className="list-disc pl-4 wrap-anywhere">
              {overlaps.map((overlap) => (
                <li key={overlap.key}>
                  {overlap.label}
                  {overlap.theirs !== undefined &&
                  overlap.yours !== undefined ? (
                    <>
                      : version {version} has {quoted(overlap.theirs)}, yours is{" "}
                      {quoted(overlap.yours)}
                    </>
                  ) : null}
                </li>
              ))}
            </ul>
          </div>
        ) : (
          <p>
            Your changes stay here until you save them on top of version{" "}
            {version}. Nothing you changed overlaps with it.
          </p>
        )}
      </AlertDescription>
    </>
  )
}
