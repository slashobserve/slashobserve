export type ScrollBox = {
  scrollHeight: number
  clientHeight: number
  scrollTop: number
}

const slack = 2

export function hasMoreBelow(box: ScrollBox): boolean {
  return box.scrollHeight - box.clientHeight - box.scrollTop > slack
}
