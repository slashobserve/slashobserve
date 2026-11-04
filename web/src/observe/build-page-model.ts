import { ApiError } from "@/api/errors"

const buildLoadFailedText = "This build could not be loaded."
const buildRequesterOnlyText = "Only the person who asked can watch this build."

function isNotFound(error: unknown): boolean {
  return error instanceof ApiError && error.status === 404
}

export function buildPageNotice({
  requestError,
  hasRequest,
  buildError,
}: {
  requestError: unknown
  hasRequest: boolean
  buildError: unknown
}): string | undefined {
  if (!hasRequest && requestError != null) {
    return isNotFound(requestError)
      ? buildRequesterOnlyText
      : buildLoadFailedText
  }
  if (isNotFound(buildError)) return buildRequesterOnlyText
  return undefined
}

export function buildPageTitle({
  specTitle,
  buildTitle,
  dashboardTitle,
}: {
  specTitle: string | undefined
  buildTitle: string | null | undefined
  dashboardTitle: string | undefined
}): string | undefined {
  return specTitle ?? buildTitle ?? dashboardTitle ?? undefined
}
