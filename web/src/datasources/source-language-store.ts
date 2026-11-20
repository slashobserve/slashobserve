import { useEffect } from "react"
import { create } from "zustand"

type Published = { uid: string; language: string } | null

type SourceLanguageState = {
  published: Published
  publish: (published: Published) => void
}

const useSourceLanguageStore = create<SourceLanguageState>((set) => ({
  published: null,
  publish: (published) => set({ published }),
}))

export function usePublishSourceLanguage(uid: string, language: string) {
  const publish = useSourceLanguageStore((state) => state.publish)
  useEffect(() => {
    publish({ uid, language })
    return () => publish(null)
  }, [uid, language, publish])
}

export function usePublishedLanguage(uid: string): string | undefined {
  return useSourceLanguageStore((state) =>
    state.published?.uid === uid ? state.published.language : undefined
  )
}
