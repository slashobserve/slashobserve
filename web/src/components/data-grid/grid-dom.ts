function firstById(
  root: Element,
  selector: string,
  attribute: string
): Map<string, Element> {
  const found = new Map<string, Element>()
  for (const element of root.querySelectorAll(selector)) {
    const id = element.getAttribute(attribute)
    if (id !== null && !found.has(id)) found.set(id, element)
  }
  return found
}

export function drawnColumns(root: Element): Map<string, Element> {
  return firstById(root, "[role=columnheader][col-id]", "col-id")
}

export function drawnRows(root: Element): Map<string, Element> {
  return firstById(root, "[role=row][row-id]", "row-id")
}
