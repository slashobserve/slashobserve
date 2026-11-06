import { downloadFile } from "@/dashboards/export-spec"

import { panelFileName } from "./csv"

async function panelToPng(element: HTMLElement): Promise<Blob> {
  const { toBlob } = await import("html-to-image")
  const backgroundColor = getComputedStyle(element)
    .getPropertyValue("--background")
    .trim()
  const blob = await toBlob(element, {
    pixelRatio: 2,
    backgroundColor: backgroundColor === "" ? undefined : backgroundColor,
  })
  if (blob === null) throw new Error("The panel could not be drawn as an image")
  return blob
}

export async function savePanelPng(element: HTMLElement, title: string) {
  downloadFile(await panelToPng(element), panelFileName(title, "png"))
}
