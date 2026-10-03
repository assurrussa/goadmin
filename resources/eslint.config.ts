import { defineConfig, globalIgnores } from 'eslint/config'
import tseslint from 'typescript-eslint'
import vueParser from 'vue-eslint-parser'
import pluginVue from 'eslint-plugin-vue'
import pluginVitest from '@vitest/eslint-plugin'
import pluginPlaywright from 'eslint-plugin-playwright'
import skipFormatting from '@vue/eslint-config-prettier/skip-formatting'

// Keep the direct upstream presets and Vue parser setup explicit. This project
// uses syntax-only linting; vue-tsc owns the separate type-check gate.
export default defineConfig(
  {
    name: 'app/files-to-lint',
    files: ['**/*.{ts,mts,tsx,vue}'],
  },

  globalIgnores(['**/dist/**', '**/dist-ssr/**', '**/coverage/**', 'scripts/**/*.cjs']),

  pluginVue.configs['flat/essential'],
  ...tseslint.configs.recommended.map((config) => ({
    ...config,
    ...('files' in config &&
      Array.isArray(config.files) && { files: [...config.files, '**/*.vue'] }),
  })),
  ...pluginVue.configs['flat/base'],
  {
    name: 'admin/vue-typescript-parser',
    files: ['*.vue', '**/*.vue'],
    languageOptions: {
      parser: vueParser,
      parserOptions: {
        parser: { js: 'espree', jsx: 'espree', ts: tseslint.parser, tsx: tseslint.parser },
        ecmaVersion: 2024,
        ecmaFeatures: { jsx: false },
        extraFileExtensions: ['.vue'],
      },
    },
    rules: {
      'vue/block-lang': ['error', { script: { lang: ['ts'], allowNoLang: false } }],
    },
  },

  {
    ...pluginVitest.configs.recommended,
    files: ['src/**/__tests__/*'],
  },

  {
    ...pluginPlaywright.configs['flat/recommended'],
    files: ['e2e/**/*.{test,spec}.{js,ts,jsx,tsx}'],
  },
  {
    name: 'admin/legacy-types',
    files: ['src/**/*.{ts,tsx,vue}'],
    rules: {
      '@typescript-eslint/no-explicit-any': 'warn',
    },
  },
  {
    name: 'admin/ui-single-word-components',
    files: ['src/js/components/ui/**/*.{vue,ts}'],
    rules: {
      'vue/multi-word-component-names': 'off',
    },
  },
  skipFormatting,
)
