import { mkdtempSync, rmSync } from 'node:fs'
import { tmpdir } from 'node:os'
import path from 'node:path'
import { createRequire } from 'node:module'
const require = createRequire(import.meta.url)
const { materializeAdminExtensions } = require('../../scripts/materialize-admin-extensions.cjs')

// Only this owned directory is written; the developer's generated host state is untouched.
export function createDemoScaffold({ manifestPaths = [] } = {}) {
  const directory = mkdtempSync(path.join(tmpdir(), 'goadmin-club-demo-'))
  try {
    const result = materializeAdminExtensions({
      generatedRoot: directory,
      manifestPaths,
      legacyRoot: directory,
      includeLegacy: false,
    })
    return {
      directory,
      sourceRoots: result.sourceRoots,
      aliases: [
        {
          find: '../../.admin-extensions/tailwind-sources.css',
          replacement: result.tailwindSourcesPath,
        },
        { find: '@admin-extensions-generated', replacement: result.generatedModulePath },
      ],
      cleanup: () => rmSync(directory, { recursive: true, force: true }),
    }
  } catch (error) {
    rmSync(directory, { recursive: true, force: true })
    throw error
  }
}
