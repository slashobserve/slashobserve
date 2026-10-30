import {
  Fragment,
  type CSSProperties,
  type ElementType,
  type ReactNode,
} from "react"
import { jsx, jsxs } from "react/jsx-runtime"
import { ExternalLinkIcon } from "lucide-react"
import { toJsxRuntime } from "hast-util-to-jsx-runtime"
import type { Components } from "react-markdown"
import rehypeParse from "rehype-parse"
import rehypeRaw from "rehype-raw"
import rehypeSanitize, { defaultSchema, type Options } from "rehype-sanitize"
import { unified, type Plugin, type PluggableList } from "unified"

import { cn } from "@/lib/utils"

import { RemoteImage } from "../remote-image"
import { isExternal } from "./safe-url"

type HastNode = Parameters<typeof toJsxRuntime>[0]
type HastRoot = Extract<HastNode, { type: "root" }>
type HastElement = Extract<HastNode, { type: "element" }>

const extraTags = [
  "center",
  "font",
  "u",
  "span",
  "small",
  "mark",
  "abbr",
  "figure",
  "figcaption",
  "section",
  "article",
  "header",
  "footer",
  "colgroup",
  "col",
  "caption",
]

const defaultAttributes = defaultSchema.attributes ?? {}
const attributesWithoutSource = { ...defaultAttributes }
delete attributesWithoutSource.source
const requiredWithoutInput = { ...defaultSchema.required }
delete requiredWithoutInput.input

const textSchema: Options = {
  ...defaultSchema,
  tagNames: [
    ...new Set([
      ...(defaultSchema.tagNames ?? []).filter((tag) => tag !== "source"),
      ...extraTags,
    ]),
  ],
  strip: ["script", "style", "template", "noscript"],
  required: requiredWithoutInput,
  attributes: {
    ...attributesWithoutSource,
    "*": [
      ...(defaultAttributes["*"] ?? []),
      "title",
      "align",
      "dir",
      "lang",
      "style",
    ],
    font: ["color", "size", "face"],
    img: ["src", "alt", "title", "width", "height"],
    td: ["colSpan", "rowSpan", "align", "vAlign", "width"],
    th: ["colSpan", "rowSpan", "align", "vAlign", "width"],
    table: ["width", "border", "cellPadding", "cellSpacing"],
    col: ["span", "width"],
    colgroup: ["span", "width"],
  },
  protocols: {
    href: ["http", "https", "mailto"],
    src: ["http", "https"],
    cite: ["http", "https"],
  },
}

const sides = ["top", "right", "bottom", "left"]
const corners = ["top-left", "top-right", "bottom-right", "bottom-left"]

function withSides(property: string): string[] {
  return [property, ...sides.map((side) => `${property}-${side}`)]
}

function withSidesAndSuffix(property: string, suffix: string): string[] {
  return [
    `${property}-${suffix}`,
    ...sides.map((side) => `${property}-${side}-${suffix}`),
  ]
}

const allowedStyleProperties = new Set([
  "color",
  "background-color",
  "font-weight",
  "font-style",
  "font-size",
  "font-family",
  "font-variant",
  "text-align",
  "text-decoration",
  "text-transform",
  "text-indent",
  "vertical-align",
  "line-height",
  "letter-spacing",
  "white-space",
  "word-break",
  "overflow-wrap",
  ...withSides("margin"),
  ...withSides("padding"),
  ...withSides("border"),
  ...withSidesAndSuffix("border", "width"),
  ...withSidesAndSuffix("border", "style"),
  ...withSidesAndSuffix("border", "color"),
  "border-radius",
  ...corners.map((corner) => `border-${corner}-radius`),
  "border-collapse",
  "border-spacing",
  "width",
  "min-width",
  "max-width",
  "height",
  "min-height",
  "max-height",
  "display",
  "flex",
  "flex-direction",
  "flex-wrap",
  "justify-content",
  "align-items",
  "gap",
  "list-style-type",
  "opacity",
])

const allowedDisplayValues = new Set([
  "block",
  "inline",
  "inline-block",
  "flex",
  "grid",
  "table",
  "table-row",
  "table-cell",
  "none",
])

const forbiddenStyleValue =
  /url\(|image\(|image-set\(|expression\(|var\(|env\(|attr\(|\\|@|\/\*|<|>/i

function allowedDeclaration(property: string, value: string): boolean {
  if (!allowedStyleProperties.has(property)) return false
  if (value === "" || forbiddenStyleValue.test(value)) return false
  if (property === "display") {
    return allowedDisplayValues.has(value.toLowerCase())
  }
  return true
}

function cleanStyle(style: string): string {
  return style
    .split(";")
    .flatMap((declaration) => {
      const colon = declaration.indexOf(":")
      if (colon < 0) return []
      const property = declaration.slice(0, colon).trim().toLowerCase()
      const value = declaration.slice(colon + 1).trim()
      return allowedDeclaration(property, value)
        ? [`${property}: ${value}`]
        : []
    })
    .join("; ")
}

function isParent(node: HastNode): node is HastRoot | HastElement {
  return node.type === "root" || node.type === "element"
}

const sanitizeStyles: Plugin<[], HastRoot> = () => (tree) => {
  const visit = (node: HastNode) => {
    if (node.type === "element" && typeof node.properties.style === "string") {
      const style = cleanStyle(node.properties.style)
      if (style === "") delete node.properties.style
      else node.properties.style = style
    }
    if (isParent(node)) node.children.forEach(visit)
  }
  visit(tree)
}

function isTaskCheckbox(node: HastNode): boolean {
  return (
    node.type === "element" &&
    node.tagName === "input" &&
    node.properties.type === "checkbox" &&
    node.properties.disabled === true
  )
}

const keepOnlyTaskCheckboxes: Plugin<[], HastRoot> = () => (tree) => {
  const visit = (node: HastNode) => {
    if (!isParent(node)) return
    node.children = node.children.filter(
      (child) =>
        !(child.type === "element" && child.tagName === "input") ||
        isTaskCheckbox(child)
    )
    node.children.forEach(visit)
  }
  visit(tree)
}

export const markdownRehypePlugins: PluggableList = [
  rehypeRaw,
  [rehypeSanitize, textSchema],
  keepOnlyTaskCheckboxes,
  sanitizeStyles,
]

const htmlProcessor = unified()
  .use(rehypeParse, { fragment: true })
  .use(rehypeSanitize, textSchema)
  .use(keepOnlyTaskCheckboxes)
  .use(sanitizeStyles)

const heading = "mt-2 font-semibold"
const block = "my-2 first:mt-0 last:mb-0"

type WithNode = { node?: unknown }

function withoutNode<P extends WithNode>(props: P): Omit<P, "node"> {
  const forwarded = { ...props }
  delete forwarded.node
  return forwarded
}

function styled(Tag: ElementType, className: string) {
  return function StyledElement(props: WithNode) {
    return <Tag {...withoutNode(props)} className={className} />
  }
}

function dimension(value: unknown): number | string | undefined {
  if (typeof value === "number") return value
  if (typeof value !== "string") return undefined
  if (/^\d+$/.test(value)) return Number(value)
  return /^\d+%$/.test(value) ? value : undefined
}

export const components: Components = {
  a({ href, title, style, children }) {
    if (!href) return <span style={style}>{children}</span>
    const external = isExternal(href)
    return (
      <a
        href={href}
        title={title}
        style={style}
        rel="noopener noreferrer"
        target={external ? "_blank" : undefined}
        className="underline underline-offset-2"
      >
        {children}
        {external ? (
          <ExternalLinkIcon
            aria-hidden="true"
            className="ml-0.5 inline size-3 align-baseline"
          />
        ) : null}
      </a>
    )
  },
  img({ src, alt, width, height }) {
    if (typeof src !== "string") return <>{alt}</>
    const size: CSSProperties = {
      width: dimension(width),
      height: dimension(height),
    }
    return (
      <RemoteImage
        src={src}
        alt={alt ?? ""}
        className="max-w-full"
        style={size}
      />
    )
  },
  h1: styled("h1", cn(heading, "text-[18px]")),
  h2: styled("h2", cn(heading, "text-[16px]")),
  h3: styled("h3", cn(heading, "text-[14px]")),
  h4: styled("h4", cn(heading, "text-[14px]")),
  h5: styled("h5", cn(heading, "text-[14px]")),
  h6: styled("h6", cn(heading, "text-[14px]")),
  p: styled("p", cn(block, "text-[14px] leading-normal")),
  ul: styled("ul", cn(block, "list-disc pl-[18px]")),
  ol: styled("ol", cn(block, "list-decimal pl-[18px]")),
  code: styled("code", "font-mono text-xs"),
  pre: styled("pre", cn(block, "overflow-auto rounded-control border p-2")),
  table: (props) => (
    <div className={cn(block, "overflow-x-auto")}>
      <table {...withoutNode(props)} className="border-collapse text-[13px]" />
    </div>
  ),
  th: styled("th", "border px-2 py-1 text-left font-semibold"),
  td: styled("td", "border px-2 py-1"),
  blockquote: styled("blockquote", cn(block, "pl-3 text-muted-foreground")),
}

export function renderHtml(html: string): ReactNode {
  const tree = htmlProcessor.runSync(htmlProcessor.parse(html))
  return toJsxRuntime(tree, {
    Fragment,
    jsx,
    jsxs,
    components,
    passKeys: true,
  })
}
