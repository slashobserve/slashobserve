import js from "@eslint/js"
import globals from "globals"
import reactHooks from "eslint-plugin-react-hooks"
import reactRefresh from "eslint-plugin-react-refresh"
import tseslint from "typescript-eslint"
import { defineConfig, globalIgnores } from "eslint/config"

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
        {
          selector:
            "CallExpression[callee.name='buttonVariants']:not(CallExpression[callee.name='cn'] > CallExpression)",
          message:
            "Wrap buttonVariants() in cn() so variant classes override base classes.",
        },
        ...rawHtmlSelectors,
      ],
    },
  },
  {
    files: ["src/components/ui/**"],
    rules: { "no-restricted-syntax": ["error", ...rawHtmlSelectors] },
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
