---
version: alpha
name: SlashObserve
description: Observability workspace for humans and agents. Calm neutral surfaces, one teal accent, colour for people, status and data.
colors:
  primary: "#0a7f86"
  primary-hover: "#086a70"
  on-primary: "#ffffff"
  primary-container: "#e0f4f4"
  on-primary-container: "#086a70"
  surface: "#ffffff"
  surface-raised: "#ffffff"
  surface-nav-active: "#ffffff"
  surface-chrome: "#f5f5f7"
  surface-canvas: "#fafafb"
  surface-muted: "#f1f1f4"
  on-surface: "#0d0d12"
  on-surface-muted: "#5d5d68"
  outline: "#e4e4e9"
  chart-grid: "#ececf0"
  success: "#3d7a00"
  success-container: "#eaf5dc"
  warning: "#8a6100"
  warning-container: "#fbf0d6"
  error: "#c22f2f"
  error-container: "#fbe8e8"
  series-1: "#2a78d6"
  series-2: "#eb6834"
  series-3: "#19a472"
  series-4: "#c28400"
  series-5: "#c2409b"
  series-6: "#7b61d1"
  series-7: "#7c8594"
  series-8: "#9a6b3f"
  person-violet: "#e7e5fb"
  on-person-violet: "#3f39a8"
  person-rose: "#fde2ea"
  on-person-rose: "#9b1c45"
  person-amber: "#fdebc8"
  on-person-amber: "#7a4b00"
  person-sky: "#dbebfb"
  on-person-sky: "#14508f"
  person-lime: "#e3f2d1"
  on-person-lime: "#3a6212"
  person-plum: "#f3e0f5"
  on-person-plum: "#7a2585"
  primary-dark: "#2ab3b1"
  primary-hover-dark: "#40c4c1"
  on-primary-dark: "#04191a"
  primary-container-dark: "#12302f"
  on-primary-container-dark: "#5fd6d0"
  surface-dark: "#131417"
  surface-raised-dark: "#1a1b1f"
  surface-nav-active-dark: "#212327"
  surface-chrome-dark: "#0b0c0e"
  surface-canvas-dark: "#0f1012"
  surface-muted-dark: "#25272c"
  on-surface-dark: "#ececf1"
  on-surface-muted-dark: "#a0a2ae"
  outline-dark: "#2e3036"
  chart-grid-dark: "#26282d"
  success-dark: "#86d43a"
  success-container-dark: "#1c2e10"
  warning-dark: "#fab219"
  warning-container-dark: "#3a2c0a"
  error-dark: "#ec6a6a"
  error-container-dark: "#331517"
  series-1-dark: "#4a94ee"
  series-2-dark: "#e8703c"
  series-3-dark: "#2bb583"
  series-4-dark: "#e0a01a"
  series-5-dark: "#dd66b6"
  series-6-dark: "#a08cf0"
  series-7-dark: "#939baa"
  series-8-dark: "#bd8c5e"
  person-violet-dark: "#2f2b63"
  on-person-violet-dark: "#c2bdfa"
  person-rose-dark: "#56192c"
  on-person-rose-dark: "#f9b3c7"
  person-amber-dark: "#4b3509"
  on-person-amber-dark: "#f5cf80"
  person-sky-dark: "#173a61"
  on-person-sky-dark: "#a9d0f7"
  person-lime-dark: "#2d4412"
  on-person-lime-dark: "#c3e298"
  person-plum-dark: "#491e4d"
  on-person-plum-dark: "#e8b3ee"
typography:
  headline-lg:
    fontFamily: Geist
    fontSize: 28px
    fontWeight: 600
    lineHeight: 1.2
    letterSpacing: -0.02em
  headline-md:
    fontFamily: Geist
    fontSize: 20px
    fontWeight: 600
    lineHeight: 1.3
    letterSpacing: -0.01em
  title-md:
    fontFamily: Geist
    fontSize: 16px
    fontWeight: 600
    lineHeight: 1.35
    letterSpacing: -0.01em
  title-sm:
    fontFamily: Geist
    fontSize: 14px
    fontWeight: 600
    lineHeight: 1.45
  body-lg:
    fontFamily: Geist
    fontSize: 15px
    fontWeight: 400
    lineHeight: 1.65
  body-md:
    fontFamily: Geist
    fontSize: 14px
    fontWeight: 400
    lineHeight: 1.45
  body-sm:
    fontFamily: Geist
    fontSize: 13px
    fontWeight: 400
    lineHeight: 1.45
  label-md:
    fontFamily: Geist
    fontSize: 13px
    fontWeight: 500
    lineHeight: 1.2
  label-sm:
    fontFamily: Geist
    fontSize: 12px
    fontWeight: 400
    lineHeight: 1.4
  label-xs:
    fontFamily: Geist
    fontSize: 11px
    fontWeight: 600
    lineHeight: 1.2
  metric:
    fontFamily: Geist
    fontSize: 28px
    fontWeight: 600
    lineHeight: 1.1
    letterSpacing: -0.02em
  code:
    fontFamily: Geist Mono
    fontSize: 13px
    fontWeight: 400
    lineHeight: 1.5
rounded:
  xs: 4px
  sm: 6px
  md: 10px
  full: 9999px
spacing:
  xs: 4px
  sm: 8px
  md: 12px
  lg: 16px
  xl: 24px
  "2xl": 32px
  gutter: 24px
  gutter-phone: 16px
  gutter-card: 16px
  container: 1144px
  container-doc: 1040px
  container-reading: 720px
  side-panel: 384px
  rail: 248px
  rail-collapsed: 52px
components:
  button-primary:
    backgroundColor: "{colors.primary}"
    textColor: "{colors.on-primary}"
    typography: "{typography.label-md}"
    rounded: "{rounded.sm}"
    height: "32px"
    padding: "0 12px"
  button-primary-hover:
    backgroundColor: "{colors.primary-hover}"
    textColor: "{colors.on-primary}"
  button-outline:
    backgroundColor: "{colors.surface-raised}"
    textColor: "{colors.on-surface}"
    typography: "{typography.label-md}"
    rounded: "{rounded.sm}"
    height: "32px"
    padding: "0 12px"
  button-outline-hover:
    backgroundColor: "{colors.surface-muted}"
    textColor: "{colors.on-surface}"
  button-ghost:
    backgroundColor: "transparent"
    textColor: "{colors.on-surface}"
    typography: "{typography.label-md}"
    rounded: "{rounded.sm}"
    height: "32px"
    padding: "0 12px"
  button-danger:
    backgroundColor: "{colors.surface}"
    textColor: "{colors.error}"
    typography: "{typography.label-md}"
    rounded: "{rounded.sm}"
    height: "32px"
    padding: "0 12px"
  page:
    backgroundColor: "{colors.surface}"
    textColor: "{colors.on-surface}"
    typography: "{typography.body-md}"
    padding: "{spacing.gutter}"
  sidebar:
    backgroundColor: "{colors.surface-chrome}"
    textColor: "{colors.on-surface}"
    width: "{spacing.rail}"
    padding: "{spacing.md}"
  sidebar-item-active:
    backgroundColor: "{colors.surface-nav-active}"
    textColor: "{colors.on-surface}"
    rounded: "{rounded.sm}"
    height: "32px"
  card:
    backgroundColor: "{colors.surface-raised}"
    textColor: "{colors.on-surface}"
    rounded: "{rounded.md}"
    padding: "{spacing.gutter-card}"
  card-inset:
    backgroundColor: "{colors.surface-canvas}"
    textColor: "{colors.on-surface}"
    rounded: "{rounded.md}"
    padding: "{spacing.md}"
  hint:
    backgroundColor: "{colors.surface}"
    textColor: "{colors.on-surface-muted}"
    typography: "{typography.label-sm}"
  tab-active:
    backgroundColor: "{colors.surface-muted}"
    textColor: "{colors.on-surface}"
    rounded: "{rounded.xs}"
    height: "28px"
  mention:
    backgroundColor: "{colors.primary-container}"
    textColor: "{colors.on-primary-container}"
    rounded: "{rounded.xs}"
    padding: "0 3px"
  selection:
    backgroundColor: "{colors.primary-container}"
    textColor: "{colors.on-surface}"
  unread-badge:
    backgroundColor: "{colors.primary}"
    textColor: "{colors.on-primary}"
    typography: "{typography.label-xs}"
    rounded: "{rounded.full}"
    height: "18px"
  agent-builtin:
    backgroundColor: "{colors.primary}"
    textColor: "{colors.on-primary}"
    rounded: "{rounded.full}"
    size: "24px"
  agent-outside:
    backgroundColor: "{colors.surface}"
    textColor: "{colors.on-surface}"
    rounded: "{rounded.full}"
    size: "24px"
  avatar-violet:
    backgroundColor: "{colors.person-violet}"
    textColor: "{colors.on-person-violet}"
    rounded: "{rounded.full}"
    size: "24px"
  avatar-rose:
    backgroundColor: "{colors.person-rose}"
    textColor: "{colors.on-person-rose}"
    rounded: "{rounded.full}"
    size: "24px"
  avatar-amber:
    backgroundColor: "{colors.person-amber}"
    textColor: "{colors.on-person-amber}"
    rounded: "{rounded.full}"
    size: "24px"
  avatar-sky:
    backgroundColor: "{colors.person-sky}"
    textColor: "{colors.on-person-sky}"
    rounded: "{rounded.full}"
    size: "24px"
  avatar-lime:
    backgroundColor: "{colors.person-lime}"
    textColor: "{colors.on-person-lime}"
    rounded: "{rounded.full}"
    size: "24px"
  avatar-plum:
    backgroundColor: "{colors.person-plum}"
    textColor: "{colors.on-person-plum}"
    rounded: "{rounded.full}"
    size: "24px"
  status-success:
    backgroundColor: "{colors.success-container}"
    textColor: "{colors.success}"
    rounded: "{rounded.xs}"
  status-warning:
    backgroundColor: "{colors.warning-container}"
    textColor: "{colors.warning}"
    rounded: "{rounded.xs}"
  status-error:
    backgroundColor: "{colors.error-container}"
    textColor: "{colors.error}"
    rounded: "{rounded.xs}"
  divider:
    backgroundColor: "{colors.outline}"
    height: "1px"
  chart-gridline:
    backgroundColor: "{colors.chart-grid}"
    height: "1px"
  chart-series-1:
    backgroundColor: "{colors.series-1}"
    height: "2px"
  chart-series-2:
    backgroundColor: "{colors.series-2}"
    height: "2px"
  chart-series-3:
    backgroundColor: "{colors.series-3}"
    height: "2px"
  chart-series-4:
    backgroundColor: "{colors.series-4}"
    height: "2px"
  chart-series-5:
    backgroundColor: "{colors.series-5}"
    height: "2px"
  chart-series-6:
    backgroundColor: "{colors.series-6}"
    height: "2px"
  chart-series-7:
    backgroundColor: "{colors.series-7}"
    height: "2px"
  chart-series-8:
    backgroundColor: "{colors.series-8}"
    height: "2px"
  button-primary-dark:
    backgroundColor: "{colors.primary-dark}"
    textColor: "{colors.on-primary-dark}"
    typography: "{typography.label-md}"
    rounded: "{rounded.sm}"
    height: "32px"
    padding: "0 12px"
  button-primary-hover-dark:
    backgroundColor: "{colors.primary-hover-dark}"
    textColor: "{colors.on-primary-dark}"
  button-outline-dark:
    backgroundColor: "{colors.surface-raised-dark}"
    textColor: "{colors.on-surface-dark}"
    typography: "{typography.label-md}"
    rounded: "{rounded.sm}"
    height: "32px"
    padding: "0 12px"
  button-outline-hover-dark:
    backgroundColor: "{colors.surface-muted-dark}"
    textColor: "{colors.on-surface-dark}"
  button-ghost-dark:
    backgroundColor: "transparent"
    textColor: "{colors.on-surface-dark}"
    typography: "{typography.label-md}"
    rounded: "{rounded.sm}"
    height: "32px"
    padding: "0 12px"
  button-danger-dark:
    backgroundColor: "{colors.surface-dark}"
    textColor: "{colors.error-dark}"
    typography: "{typography.label-md}"
    rounded: "{rounded.sm}"
    height: "32px"
    padding: "0 12px"
  page-dark:
    backgroundColor: "{colors.surface-dark}"
    textColor: "{colors.on-surface-dark}"
    typography: "{typography.body-md}"
    padding: "{spacing.gutter}"
  sidebar-dark:
    backgroundColor: "{colors.surface-chrome-dark}"
    textColor: "{colors.on-surface-dark}"
    width: "{spacing.rail}"
    padding: "{spacing.md}"
  sidebar-item-active-dark:
    backgroundColor: "{colors.surface-nav-active-dark}"
    textColor: "{colors.on-surface-dark}"
    rounded: "{rounded.sm}"
    height: "32px"
  card-dark:
    backgroundColor: "{colors.surface-raised-dark}"
    textColor: "{colors.on-surface-dark}"
    rounded: "{rounded.md}"
    padding: "{spacing.gutter-card}"
  card-inset-dark:
    backgroundColor: "{colors.surface-canvas-dark}"
    textColor: "{colors.on-surface-dark}"
    rounded: "{rounded.md}"
    padding: "{spacing.md}"
  hint-dark:
    backgroundColor: "{colors.surface-dark}"
    textColor: "{colors.on-surface-muted-dark}"
    typography: "{typography.label-sm}"
  tab-active-dark:
    backgroundColor: "{colors.surface-muted-dark}"
    textColor: "{colors.on-surface-dark}"
    rounded: "{rounded.xs}"
    height: "28px"
  mention-dark:
    backgroundColor: "{colors.primary-container-dark}"
    textColor: "{colors.on-primary-container-dark}"
    rounded: "{rounded.xs}"
    padding: "0 3px"
  selection-dark:
    backgroundColor: "{colors.primary-container-dark}"
    textColor: "{colors.on-surface-dark}"
  unread-badge-dark:
    backgroundColor: "{colors.primary-dark}"
    textColor: "{colors.on-primary-dark}"
    typography: "{typography.label-xs}"
    rounded: "{rounded.full}"
    height: "18px"
  agent-builtin-dark:
    backgroundColor: "{colors.primary-dark}"
    textColor: "{colors.on-primary-dark}"
    rounded: "{rounded.full}"
    size: "24px"
  agent-outside-dark:
    backgroundColor: "{colors.surface-dark}"
    textColor: "{colors.on-surface-dark}"
    rounded: "{rounded.full}"
    size: "24px"
  avatar-violet-dark:
    backgroundColor: "{colors.person-violet-dark}"
    textColor: "{colors.on-person-violet-dark}"
    rounded: "{rounded.full}"
    size: "24px"
  avatar-rose-dark:
    backgroundColor: "{colors.person-rose-dark}"
    textColor: "{colors.on-person-rose-dark}"
    rounded: "{rounded.full}"
    size: "24px"
  avatar-amber-dark:
    backgroundColor: "{colors.person-amber-dark}"
    textColor: "{colors.on-person-amber-dark}"
    rounded: "{rounded.full}"
    size: "24px"
  avatar-sky-dark:
    backgroundColor: "{colors.person-sky-dark}"
    textColor: "{colors.on-person-sky-dark}"
    rounded: "{rounded.full}"
    size: "24px"
  avatar-lime-dark:
    backgroundColor: "{colors.person-lime-dark}"
    textColor: "{colors.on-person-lime-dark}"
    rounded: "{rounded.full}"
    size: "24px"
  avatar-plum-dark:
    backgroundColor: "{colors.person-plum-dark}"
    textColor: "{colors.on-person-plum-dark}"
    rounded: "{rounded.full}"
    size: "24px"
  status-success-dark:
    backgroundColor: "{colors.success-container-dark}"
    textColor: "{colors.success-dark}"
    rounded: "{rounded.xs}"
  status-warning-dark:
    backgroundColor: "{colors.warning-container-dark}"
    textColor: "{colors.warning-dark}"
    rounded: "{rounded.xs}"
  status-error-dark:
    backgroundColor: "{colors.error-container-dark}"
    textColor: "{colors.error-dark}"
    rounded: "{rounded.xs}"
  divider-dark:
    backgroundColor: "{colors.outline-dark}"
    height: "1px"
  chart-gridline-dark:
    backgroundColor: "{colors.chart-grid-dark}"
    height: "1px"
  chart-series-1-dark:
    backgroundColor: "{colors.series-1-dark}"
    height: "2px"
  chart-series-2-dark:
    backgroundColor: "{colors.series-2-dark}"
    height: "2px"
  chart-series-3-dark:
    backgroundColor: "{colors.series-3-dark}"
    height: "2px"
  chart-series-4-dark:
    backgroundColor: "{colors.series-4-dark}"
    height: "2px"
  chart-series-5-dark:
    backgroundColor: "{colors.series-5-dark}"
    height: "2px"
  chart-series-6-dark:
    backgroundColor: "{colors.series-6-dark}"
    height: "2px"
  chart-series-7-dark:
    backgroundColor: "{colors.series-7-dark}"
    height: "2px"
  chart-series-8-dark:
    backgroundColor: "{colors.series-8-dark}"
    height: "2px"
  composer:
    backgroundColor: "{colors.surface-raised}"
    textColor: "{colors.on-surface}"
    typography: "{typography.body-md}"
    rounded: "{rounded.md}"
    padding: "{spacing.md}"
  send-button:
    backgroundColor: "{colors.primary}"
    textColor: "{colors.on-primary}"
    rounded: "{rounded.sm}"
    size: "28px"
  side-panel:
    backgroundColor: "{colors.surface-raised}"
    textColor: "{colors.on-surface}"
    rounded: "{rounded.md}"
    width: "{spacing.side-panel}"
    padding: "{spacing.gutter-card}"
  input:
    backgroundColor: "{colors.surface}"
    textColor: "{colors.on-surface}"
    typography: "{typography.body-md}"
    rounded: "{rounded.sm}"
    height: "32px"
    padding: "0 12px"
  chip:
    backgroundColor: "{colors.surface}"
    textColor: "{colors.on-surface-muted}"
    typography: "{typography.label-xs}"
    rounded: "{rounded.xs}"
    padding: "1px 6px"
  code-block:
    backgroundColor: "{colors.surface-canvas}"
    textColor: "{colors.on-surface}"
    typography: "{typography.code}"
    rounded: "{rounded.md}"
    padding: "{spacing.md}"
  page-title:
    textColor: "{colors.on-surface}"
    typography: "{typography.headline-md}"
  document-title:
    textColor: "{colors.on-surface}"
    typography: "{typography.headline-lg}"
  document-text:
    textColor: "{colors.on-surface}"
    typography: "{typography.body-lg}"
  section-title:
    textColor: "{colors.on-surface}"
    typography: "{typography.title-sm}"
  block-title:
    textColor: "{colors.on-surface}"
    typography: "{typography.title-md}"
  secondary-text:
    textColor: "{colors.on-surface}"
    typography: "{typography.body-sm}"
  stat-tile-value:
    textColor: "{colors.on-surface}"
    typography: "{typography.metric}"
---

# SlashObserve

## Overview

SlashObserve is an observability workspace where people and agents work on the same pages: dashboards, observations, logs and traces.
The interface should feel like a shared notebook, not a control room.
It is calm, flat and mostly neutral, so that the three things that matter carry the colour: who did something, what state it is in, and the data itself.

Principles:

- **Everything has an author.** Every author is a round mark. People are tinted, the built-in agent is filled with the accent, an outside agent is an ink outline, and it reads as "Ishan (via Claude Code)".
- **One accent.** Teal marks the single main action of a view and anything that is "yours to notice": a selection, a mention, an unread item, focus.
- **Colour is information.** If a colour does not say who, what state or which series, it should be neutral.
- **Flat.** White pages, 1px borders, no shadows, no gradients.
- **Write on the page.** Comments sit in the margin, suggestions are dashed until someone keeps them, and one composer serves both people and agents.

The wordmark is `/observe` in Geist 600. The slash takes the accent, the word takes the text colour.
The built-in agent is called Observe Agent in the interface. People mention it as `@observe`.

Themes: light and dark are both first class. Every colour token has a light value and a `-dark` twin (`primary` and `primary-dark`). The format has no theme mechanism, so the suffix is the convention. Light is the default, and the app follows the system setting until the person chooses.

## Colors

The palette has five families. Use the token, never the hex.

### Accent

| Token                  | Light     | Dark      | Use                                                                                           |
| ---------------------- | --------- | --------- | --------------------------------------------------------------------------------------------- |
| `primary`              | `#0a7f86` | `#2ab3b1` | The one accent: main action, the wordmark slash, the built-in agent, selection, unread, focus |
| `primary-hover`        | `#086a70` | `#40c4c1` | Hover and pressed state of the accent                                                         |
| `on-primary`           | `#ffffff` | `#04191a` | Text and icons on the accent                                                                  |
| `primary-container`    | `#e0f4f4` | `#12302f` | Mentions, highlighted text, a selected time range                                             |
| `on-primary-container` | `#086a70` | `#5fd6d0` | Accent-coloured text on a surface or on the soft accent                                       |

Use the accent for one filled button per page or card, the send button in a composer, links that start an agent run, and selection. Do not use it for decoration, headings or large fills.

### Surfaces and text

| Token                | Light     | Dark      | Use                                                                                        |
| -------------------- | --------- | --------- | ------------------------------------------------------------------------------------------ |
| `surface`            | `#ffffff` | `#131417` | The page                                                                                   |
| `surface-raised`     | `#ffffff` | `#1a1b1f` | Cards, panels, menus, outline buttons. Same as the page in light, one step lighter in dark |
| `surface-chrome`     | `#f5f5f7` | `#0b0c0e` | The sidebar                                                                                |
| `surface-nav-active` | `#ffffff` | `#212327` | The active sidebar item                                                                    |
| `surface-canvas`     | `#fafafb` | `#0f1012` | Inset areas: code, a hovered row                                                           |
| `surface-muted`      | `#f1f1f4` | `#25272c` | Active tab, hover fill                                                                     |
| `on-surface`         | `#0d0d12` | `#ececf1` | Text                                                                                       |
| `on-surface-muted`   | `#5d5d68` | `#a0a2ae` | Hints and secondary text                                                                   |
| `outline`            | `#e4e4e9` | `#2e3036` | Every 1px border                                                                           |
| `chart-grid`         | `#ececf0` | `#26282d` | Chart gridlines                                                                            |

Body text on any surface meets WCAG AA (4.5:1). Hints use `on-surface-muted`, which also meets AA on `surface`, `surface-chrome` and `surface-muted`.

### Status

| Token               | Light     | Dark      | Use                     |
| ------------------- | --------- | --------- | ----------------------- |
| `success`           | `#3d7a00` | `#86d43a` | Healthy, saved, allowed |
| `success-container` | `#eaf5dc` | `#1c2e10` | Background behind it    |
| `warning`           | `#8a6100` | `#fab219` | Waiting, needs a choice |
| `warning-container` | `#fbf0d6` | `#3a2c0a` | Background behind it    |
| `error`             | `#c22f2f` | `#ec6a6a` | Failed, destructive     |
| `error-container`   | `#fbe8e8` | `#331517` | Background behind it    |

Status always comes as an icon and a word, never as colour alone. Use the strong colour for the icon or the text, and the container colour only for a small badge behind it.

### Dark theme

Dark is its own palette, not the light one inverted.

- **Layers by lightness.** With no shadows, depth in dark comes from four steps: the sidebar is the darkest (`surface-chrome-dark`), then the page (`surface-dark`), then cards, panels and menus one step lighter (`surface-raised-dark`), and hover or active fills lighter again (`surface-muted-dark`). Inset areas such as code go darker than the page (`surface-canvas-dark`).
- **Softer text.** Text is an off-white, never pure white, and borders are a visible grey so cards hold their edge.
- **A calmer accent.** Filled buttons use a mid teal with near-black text. The brighter teal is kept for small things: links, the unread dot, a selection edge.
- **Brighter data.** Chart series and status colours are lifted so lines stay readable on a dark card. People tints become deep fills with light initials.
- Form controls and scrollbars use the dark colour scheme too.

### Chart series

| Token      | Light     | Dark      | Use          |
| ---------- | --------- | --------- | ------------ |
| `series-1` | `#2a78d6` | `#4a94ee` | First series |
| `series-2` | `#eb6834` | `#e8703c` | Second       |
| `series-3` | `#19a472` | `#2bb583` | Third        |
| `series-4` | `#c28400` | `#e0a01a` | Fourth       |
| `series-5` | `#c2409b` | `#dd66b6` | Fifth        |
| `series-6` | `#7b61d1` | `#a08cf0` | Sixth        |
| `series-7` | `#7c8594` | `#939baa` | Seventh      |
| `series-8` | `#9a6b3f` | `#bd8c5e` | Eighth       |

Series colours belong to data only: lines, bars, legend swatches. Assign them in order and never reuse them for buttons, status or people. Blue is `series-1`, which is why the accent is not blue. Status green leans yellow so it does not read as the teal accent.
Light series 3 and 4 are darker than the canvas values so every series keeps 3:1 contrast on a white card.
A chart never relies on colour alone: every series has a legend label.

### People

| Tint            | Light fill and text     | Dark fill and text      |
| --------------- | ----------------------- | ----------------------- |
| `person-violet` | `#e7e5fb` and `#3f39a8` | `#2f2b63` and `#c2bdfa` |
| `person-rose`   | `#fde2ea` and `#9b1c45` | `#56192c` and `#f9b3c7` |
| `person-amber`  | `#fdebc8` and `#7a4b00` | `#4b3509` and `#f5cf80` |
| `person-sky`    | `#dbebfb` and `#14508f` | `#173a61` and `#a9d0f7` |
| `person-lime`   | `#e3f2d1` and `#3a6212` | `#2d4412` and `#c3e298` |
| `person-plum`   | `#f3e0f5` and `#7a2585` | `#491e4d` and `#e8b3ee` |

Each person gets one tint for their avatar, assigned in order when they join and kept everywhere. The tint is used for the avatar only. Agents never take a person tint.

## Typography

Geist for everything, Geist Mono for queries, JSON, ids and log lines.

| Token         | Size and weight | Use                                                    |
| ------------- | --------------- | ------------------------------------------------------ |
| `headline-lg` | 28, 600         | Title of a document page: an observation               |
| `headline-md` | 20, 600         | Title of an app page: Home, Inbox, Agents, a dashboard |
| `title-md`    | 16, 600         | A block or note title inside a list or a page          |
| `title-sm`    | 14, 600         | Section and card headings                              |
| `body-lg`     | 15, 400         | Text people write and read on a document page          |
| `body-md`     | 14, 400         | Default interface text                                 |
| `body-sm`     | 13, 400         | Secondary text, table cells, comments                  |
| `label-md`    | 13, 500         | Buttons, tabs, navigation                              |
| `label-sm`    | 12, 400         | Hints, timestamps, metadata                            |
| `label-xs`    | 11, 600         | Counts, chips, avatar initials                         |
| `metric`      | 28, 600         | The number on a stat tile, with tabular figures        |
| `code`        | 13, 400, mono   | Queries, diffs, log lines                              |

Sentence case everywhere. No uppercase labels. Weight carries hierarchy: 600 for titles and names, 500 for controls, 400 for the rest.

## Layout

A 4px base. The spacing scale is 4, 8, 12, 16, 24, 32.

- **Gutters.** The page gutter is `gutter` (24px), and `gutter-phone` (16px) under 900px. The top bar, the page title and the content share that left edge. Inside cards and panels the gutter is `gutter-card` (16px).
- **Widths.** App pages fill the content width, as the dashboards list does. Home and the chat pages centre a `container-reading` (720px) column. The side panel is always `side-panel` (384px).
- **Shell.** A grey sidebar (`rail`, 248px) and a white page. The page has a top bar with the breadcrumb on the left and page actions on the right.
- **Control heights.** 32px for page actions, 28px inside cards and panels, 24px for dense icon buttons, 44px for touch targets on phones.

Breakpoints:

| Width        | What changes                                                                                                        |
| ------------ | ------------------------------------------------------------------------------------------------------------------- |
| Under 1200px | The sidebar folds to an icon rail (`rail-collapsed`, 52px)                                                          |
| Under 1100px | The side panel and margin comments drop below the content. Inbox shows the list alone                               |
| Under 900px  | The sidebar becomes a top bar. Stat tiles go two across, panels go full width, floating notes sit under their chart |

No page scrolls sideways at 390px. Tables scroll inside their own container.

## Elevation & Depth

There are no shadows. In light, depth comes from a 1px `outline` border and from the grey sidebar against the white page. In dark, it comes from the border and from the surface steps described under Colors.
No shadows, no blur, no overlays that dim the page. Menus and popovers are cards with a border.
Emphasis uses a stronger border: a selected panel or the active composer takes a 1px `on-surface` border, a selection takes the accent.

## Shapes

| Token  | Value | Use                                                                   |
| ------ | ----- | --------------------------------------------------------------------- |
| `xs`   | 4px   | Chips, tags, keyboard hints, mentions, the active segment of a toggle |
| `sm`   | 6px   | Buttons, inputs, navigation items                                     |
| `md`   | 10px  | Cards, panels, composers, menus                                       |
| `full` | round | People and agent avatars, counts, switches, status dots               |

Round means an author (a person or an agent) or a count. A 6px corner means a control. Do not mix them.
A dashed 1.5px border means "suggested, not saved yet". A solid border means saved.

## Components

- **Buttons.** Three types. `button-primary` is the one main action of a page or a card. `button-outline` is every other action. `button-ghost` is for quiet actions in toolbars and rows. `button-danger` is an outline button with error-coloured text. Page actions sit in the top bar on the right.
- **Composer.** One composer everywhere. Plain text is a comment, typing @ picks an agent and starts a run. It always sends with `send-button`, a 28px square accent button with an arrow. When replying, the quoted message sits above the text.
- **Avatars and agent marks.** `avatar-*` for people, `agent-builtin` for Observe Agent, `agent-outside` for connected agents. Sizes are 20, 24 and 32. A byline names the person behind an agent: "Observe Agent for Priya", "Ishan (via Claude Code)".
- **Mentions.** `mention` is the soft accent with accent text, for people and agents alike.
- **Selection.** A selected chart range or highlighted sentence takes `selection` with an accent edge. A small toolbar appears beside it: Comment, Ask @observe, Snapshot.
- **Suggestions.** A change proposed by an agent or a person appears in place with a dashed border and two buttons, Keep (primary) and Discard (outline).
- **Cards and panels.** `card` for content, `card-inset` for code and diffs, `side-panel` for the discussion or an editor beside the page.
- **Tabs and toggles.** The active item takes `tab-active`. Tabs are never filled with the accent.
- **Unread.** An accent dot before the item and `unread-badge` for a count.
- **Status.** `status-success`, `status-warning` and `status-error`, always with an icon and a word.
- **Charts.** Lines 1.6px in `chart-series-*`, gridlines in `chart-gridline`, legends in mono `label-sm`.

Dark components carry the `-dark` suffix and use the `-dark` colours.

## Do's and Don'ts

Do:

- Keep one filled accent button per page or card.
- Give every item an author mark and a byline.
- Pair every status colour with an icon and a word.
- Use the tokens for every colour, corner and gutter.
- Check both themes.

Don't:

- Don't use a series colour for anything that is not data, or the accent for data.
- Don't use shadows, gradients, blur or coloured left-border callouts.
- Don't use pills for buttons or tags. Round is for people and counts.
- Don't use emoji, uppercase labels or more than two weights in one line.
- Don't tint agents with person colours, or draw any author as a square.
- Don't invent a fourth button type or a new corner size.
