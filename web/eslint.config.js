// file: web/eslint.config.js
// version: 1.1.0
// guid: 456e7890-b12c-34d5-c678-901234567890

import js from '@eslint/js';
import typescript from '@typescript-eslint/eslint-plugin';
import typescriptParser from '@typescript-eslint/parser';
import reactHooks from 'eslint-plugin-react-hooks';
import reactRefresh from 'eslint-plugin-react-refresh';
import globals from 'globals';

const sharedGlobals = {
  ...globals.browser,
  ...globals.es2020,
  ...globals.node,
  React: 'readonly',
  RequestInit: 'readonly',
  HeadersInit: 'readonly',
  process: 'readonly',
  __dirname: 'readonly',
  global: 'readonly',
};

const baseJsConfig = {
  ...js.configs.recommended,
  rules: {
    ...js.configs.recommended.rules,
    // TypeScript handles globals/types; disable to avoid false positives.
    'no-undef': 'off',
  },
};

export default [
  {
    ignores: ['dist/**', 'node_modules/**'],
  },
  baseJsConfig,
  {
    files: ['**/*.{ts,tsx}'],
    languageOptions: {
      parser: typescriptParser,
      parserOptions: {
        ecmaVersion: 2020,
        sourceType: 'module',
        ecmaFeatures: {
          jsx: true,
        },
      },
      globals: sharedGlobals,
    },
    plugins: {
      '@typescript-eslint': typescript,
      'react-hooks': reactHooks,
      'react-refresh': reactRefresh,
    },
    rules: {
      ...typescript.configs.recommended.rules,
      ...reactHooks.configs.recommended.rules,
      'react-refresh/only-export-components': [
        'warn',
        { allowConstantExport: true },
      ],
      '@typescript-eslint/no-explicit-any': 'warn',
      '@typescript-eslint/no-unused-vars': [
        'error',
        {
          argsIgnorePattern: '^_',
          varsIgnorePattern: '^_',
        },
      ],
    },
  },
  {
    files: ['**/*'],
    languageOptions: {
      globals: {
        React: 'readonly',
        RequestInit: 'readonly',
        HeadersInit: 'readonly',
        process: 'readonly',
        __dirname: 'readonly',
        global: 'readonly',
      },
    },
    rules: {
      'no-undef': 'off',
    },
  },
];
