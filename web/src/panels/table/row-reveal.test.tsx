import { fireEvent, render } from "@testing-library/react"
import { useRef } from "react"
import { afterEach, describe, expect, it, vi } from "vitest"

import { useRowReveal } from "./row-reveal"

type Row = { key: string }

function Rows({
  rows,
  drawn,
}: {
  rows: readonly Row[]
  drawn: readonly string[]
}) {
  const region = useRef<HTMLDivElement>(null)
  const reveal = useRowReveal(region, rows)
  return (
    <div ref={region}>
      {["c", "z"].map((key) => (
        <button key={key} type="button" onClick={() => reveal(key)}>
          {`Reveal ${key}`}
        </button>
      ))}
      {drawn.map((key) => (
        <div key={key} data-row-key={key} />
      ))}
    </div>
  )
}

const scrolled = vi.fn()

function watchScrolls() {
  scrolled.mockClear()
  Element.prototype.scrollIntoView = function (this: Element) {
    scrolled((this as HTMLElement).dataset.rowKey)
  }
}

afterEach(() => {
  delete (Element.prototype as Partial<Element>).scrollIntoView
})

const all = [{ key: "a" }, { key: "b" }, { key: "c" }]

describe("useRowReveal", () => {
  it("scrolls the row into view once it is drawn", () => {
    watchScrolls()
    const view = render(<Rows rows={all} drawn={["a"]} />)
    fireEvent.click(view.getByRole("button", { name: "Reveal c" }))
    expect(scrolled).not.toHaveBeenCalled()
    view.rerender(<Rows rows={all} drawn={["b", "c"]} />)
    expect(scrolled).toHaveBeenCalledWith("c")
    view.rerender(<Rows rows={all} drawn={["c"]} />)
    expect(scrolled).toHaveBeenCalledTimes(1)
  })

  it("forgets a row it never drew once the rows change, so a later scroll does not jump to it", () => {
    watchScrolls()
    const view = render(<Rows rows={all} drawn={["a"]} />)
    fireEvent.click(view.getByRole("button", { name: "Reveal c" }))
    const filtered = [{ key: "a" }]
    view.rerender(<Rows rows={filtered} drawn={["a"]} />)
    view.rerender(<Rows rows={all} drawn={["a", "b", "c"]} />)
    expect(scrolled).not.toHaveBeenCalled()
  })

  it("does not wait for a row that is not in the table", () => {
    watchScrolls()
    const view = render(<Rows rows={all} drawn={["a"]} />)
    fireEvent.click(view.getByRole("button", { name: "Reveal z" }))
    view.rerender(<Rows rows={all} drawn={["a", "z"]} />)
    expect(scrolled).not.toHaveBeenCalled()
  })
})
