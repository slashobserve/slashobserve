export class RendererLoadError extends Error {}

export function guardLoad<T>(load: () => Promise<T>): () => Promise<T> {
  return () =>
    load().catch((error: unknown) => {
      throw new RendererLoadError(
        error instanceof Error ? error.message : "Could not load the renderer",
        { cause: error }
      )
    })
}
