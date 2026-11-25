import js from "@eslint/js"
import globals from "globals"
import reactHooks from "eslint-plugin-react-hooks"
import reactRefresh from "eslint-plugin-react-refresh"
import tseslint from "typescript-eslint"
import { defineConfig, globalIgnores } from "eslint/config"

const nativeSelectSelector = {
  selector: "JSXOpeningElement[name.name='select']",
  message:
    "Dropdowns use Select from @/components/ui/select (or Combobox for long lists), never a native select.",
}

const rawHtmlSelectors = [
  {
    selector: "JSXAttribute[name.name='dangerouslySetInnerHTML']",
    message:
      "HTML reaches the page only through the text panel's sanitizer, as React elements.",
  },
  {
    selector:
      "AssignmentExpression > MemberExpression[property.name=/^(innerHTML|outerHTML)$/]",
    message:
      "HTML reaches the page only through the text panel's sanitizer, as React elements.",
  },
  {
    selector: "CallExpression[callee.property.name='insertAdjacentHTML']",
    message:
      "HTML reaches the page only through the text panel's sanitizer, as React elements.",
  },
]

const buttonVariantsSelector = {
  selector:
    "CallExpression[callee.name='buttonVariants']:not(CallExpression[callee.name='cn'] > CallExpression)",
  message:
    "Wrap buttonVariants() in cn() so variant classes override base classes.",
}

const tableElementSelector = {
  selector: "JSXOpeningElement[name.name='table']",
  message: "Tables are a DataGrid from @/components/data-grid/data-grid.",
}

const tableSetupImports = [
  "@/components/table-card",
  "@/components/ui/table",
  "@tanstack/react-table",
].map((name) => ({
  name,
  message: "Tables are a DataGrid from @/components/data-grid/data-grid.",
}))

const enterpriseGridImports = {
  group: [
    "ag-grid-enterprise",
    "ag-grid-enterprise/*",
    "@ag-grid-enterprise/*",
  ],
  message: "Only AG Grid Community is used; no Enterprise module, ever.",
}

const gridImports = {
  group: ["ag-grid-*", "@ag-grid-*/*"],
  message:
    "Only @/components/data-grid imports AG Grid; pass columns to DataGrid.",
}

const panelTimeImports = {
  name: "@/lib/time",
  importNames: ["formatDateTime", "formatDateTimeMs", "formatTimeValue"],
  message: "Panels format times with zonedDateTime and the panel's zone.",
}

function restrictedImports({ tables, panelTime }) {
  return [
    "error",
    {
      paths: [
        ...(tables ? tableSetupImports : []),
        ...(panelTime ? [panelTimeImports] : []),
      ],
      patterns: [enterpriseGridImports, gridImports],
    },
  ]
}

const notTables = [
  "src/components/date-calendar.tsx",
  "src/panels/text/html.tsx",
  "src/panels/text/html.test.ts",
]

const waitingForDataGrid = [
  "src/agents/connected-table.tsx",
  "src/agents/service-accounts-table.tsx",
  "src/chat/chats-page.tsx",
  "src/components/table-card.test.tsx",
  "src/components/table-card.tsx",
  "src/components/ui/table.tsx",
  "src/connected-agents/connected-agents-card.tsx",
  "src/dashboards/dashboard-columns.test.tsx",
  "src/dashboards/dashboard-columns.tsx",
  "src/dashboards/dashboard-list.tsx",
  "src/datasources/datasource-table.tsx",
  "src/datasources/explore-list.tsx",
  "src/folders/folder-observations.tsx",
  "src/folders/folder-saved-searches.tsx",
  "src/history/version-list.tsx",
  "src/import/datasources-card.tsx",
  "src/import/repair/repair-page.tsx",
  "src/inspect/data-tab.tsx",
  "src/logs/patterns-view.tsx",
  "src/logs/plain-table.tsx",
  "src/observations/observation-list.tsx",
  "src/panels/legend/legend-table.tsx",
  "src/panels/table/table-panel.tsx",
  "src/people/invites-table.tsx",
  "src/people/people-table.tsx",
  "src/people/teams-card.tsx",
  "src/saved-searches/saved-search-table.tsx",
  "src/service-accounts/accounts-table.tsx",
  "src/service-accounts/keys-table.tsx",
  "src/settings/grafana/connections-card.tsx",
  "src/settings/grafana/import-launcher.tsx",
  "src/settings/webhooks/webhooks-card.tsx",
]

const tableExempt = [...notTables, ...waitingForDataGrid]

export default defineConfig([
  globalIgnores([
    "dist",
    "playwright-report",
    "test-results",
    "src/routeTree.gen.ts",
    "src/spec/*.gen.ts",
    "src/api/*.gen.ts",
  ]),
  {
    files: ["src/**/*.{ts,tsx}"],
    extends: [
      js.configs.recommended,
      tseslint.configs.recommended,
      reactHooks.configs.flat.recommended,
      reactRefresh.configs.vite,
    ],
    languageOptions: { globals: globals.browser },
    rules: {
      "react-refresh/only-export-components": "off",
      "no-restricted-syntax": [
        "error",
        buttonVariantsSelector,
        nativeSelectSelector,
        tableElementSelector,
        ...rawHtmlSelectors,
      ],
      "no-restricted-imports": restrictedImports({
        tables: true,
        panelTime: false,
      }),
    },
  },
  {
    files: ["src/panels/**/*.{ts,tsx}"],
    ignores: ["src/panels/**/*.test.{ts,tsx}"],
    rules: {
      "no-restricted-imports": restrictedImports({
        tables: true,
        panelTime: true,
      }),
    },
  },
  {
    files: ["src/components/ui/**"],
    rules: {
      "no-restricted-syntax": [
        "error",
        nativeSelectSelector,
        tableElementSelector,
        ...rawHtmlSelectors,
      ],
    },
  },
  {
    files: tableExempt,
    rules: {
      "no-restricted-syntax": [
        "error",
        buttonVariantsSelector,
        nativeSelectSelector,
        ...rawHtmlSelectors,
      ],
      "no-restricted-imports": restrictedImports({
        tables: false,
        panelTime: false,
      }),
    },
  },
  {
    files: tableExempt.filter(
      (file) => file.startsWith("src/panels/") && !/\.test\.tsx?$/.test(file)
    ),
    rules: {
      "no-restricted-imports": restrictedImports({
        tables: false,
        panelTime: true,
      }),
    },
  },
  {
    files: ["src/components/data-grid/**"],
    rules: {
      "no-restricted-imports": [
        "error",
        { paths: tableSetupImports, patterns: [enterpriseGridImports] },
      ],
    },
  },
  {
    files: [
      "e2e/**/*.ts",
      "playwright.config.ts",
      "vite.config.ts",
      "vitest.config.ts",
    ],
    extends: [js.configs.recommended, tseslint.configs.recommended],
    languageOptions: { globals: globals.node },
  },
  {
    files: ["**/*.{js,mjs}"],
    extends: [js.configs.recommended],
    languageOptions: { globals: globals.node },
  },
  {
    files: ["public/**/*.js"],
    extends: [js.configs.recommended],
    languageOptions: { globals: globals.browser, sourceType: "script" },
  },
])
