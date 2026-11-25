import { themeQuartz } from "ag-grid-community"

export const gridTheme = themeQuartz.withParams({
  fontFamily: "inherit",
  fontSize: 13,
  dataFontSize: 13,
  foregroundColor: "var(--foreground)",
  textColor: "var(--foreground)",
  subtleTextColor: "var(--muted-foreground)",
  backgroundColor: "var(--card)",
  chromeBackgroundColor: "var(--card)",
  headerBackgroundColor: "var(--card)",
  headerTextColor: "var(--muted-foreground)",
  headerFontWeight: 500,
  headerFontSize: 13,
  borderColor: "var(--border)",
  wrapperBorder: { color: "var(--border)" },
  wrapperBorderRadius: 10,
  borderRadius: 6,
  rowBorder: { color: "var(--border)" },
  headerRowBorder: { color: "var(--border)" },
  columnBorder: false,
  headerColumnBorder: false,
  headerColumnResizeHandleColor: "transparent",
  rowHoverColor: "color-mix(in srgb, var(--muted) 40%, transparent)",
  selectedRowBackgroundColor: "var(--selection)",
  accentColor: "var(--primary)",
  rangeSelectionBorderColor: "var(--ring)",
  focusShadow: { radius: 2, spread: 0, color: "var(--ring)" },
  cellHorizontalPadding: 14,
  iconColor: "var(--muted-foreground)",
  scrollbarThumbColor: "var(--border)",
  autoHeightMinBodyHeight: 0,
})

export const panelTableTheme = gridTheme.withParams({
  fontSize: 12,
  dataFontSize: 12,
  headerFontSize: 12,
  wrapperBorder: false,
  wrapperBorderRadius: 0,
  cellHorizontalPadding: 0,
  pinnedColumnBorder: false,
  pinnedRowBorder: { color: "var(--border)" },
})

export const legendTableTheme = panelTableTheme.withParams({
  backgroundColor: "transparent",
  chromeBackgroundColor: "transparent",
  headerBackgroundColor: "transparent",
  rowBorder: false,
  headerRowBorder: false,
  rowHoverColor: "transparent",
})
