export function pairedAt<T>(items: ArrayLike<T>, index: number): T {
  const item = items[index]
  if (item === undefined) {
    throw new RangeError(`No entry at ${index} in a parallel array`)
  }
  return item
}
