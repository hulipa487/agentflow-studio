import { describe, expect, it } from 'vitest'
import type { PluginsList } from '@/api/types'
import {
  PACKAGE_NAME_RE,
  defaultSelection,
  insertPreludeDirective,
  isValidScriptFilename,
  normalizeScriptFilename,
  packageStarterSource,
  selectionExists,
  selectionKey,
} from './helpers'

const fixtureList: PluginsList = {
  ok: true,
  mode: 'configdir',
  editable: true,
  plugin_dir: '/etc/agentflow/plugins',
  plugin_dir_configured: true,
  prelude_version: 2,
  builtins: [
    {
      name: 'token_budget',
      kind: 'support',
      summary: 'budget helper',
      shadowed: false,
      source: '// af-prelude-version: 2\n',
      builtin_source: '// af-prelude-version: 2\n',
    },
  ],
  packages: [
    {
      name: 'shared',
      dir: '/etc/agentflow/packages/shared',
      managed: true,
      files: [{ name: 'init.ts', path: '/etc/agentflow/packages/shared/init.ts', source: '// af-prelude-version: 2\n' }],
    },
  ],
}

describe('plugins helpers', () => {
  it('validates package and file names', () => {
    expect(PACKAGE_NAME_RE.test('shared-helpers_v1')).toBe(true)
    expect(PACKAGE_NAME_RE.test('../bad')).toBe(false)
    expect(isValidScriptFilename('helpers.ts')).toBe(true)
    expect(isValidScriptFilename('helpers.js')).toBe(true)
    expect(isValidScriptFilename('types.d.ts')).toBe(false)
    expect(isValidScriptFilename('helpers.txt')).toBe(false)
    expect(normalizeScriptFilename('util')).toBe('util.ts')
    expect(normalizeScriptFilename('util.ts')).toBe('util.ts')
    expect(normalizeScriptFilename('util.js')).toBe('util.js')
  })

  it('tracks selection existence and defaults', () => {
    const def = defaultSelection(fixtureList)
    expect(selectionKey(def)).toBe('builtin:token_budget')
    expect(selectionExists(fixtureList, def)).toBe(true)
    expect(selectionExists(fixtureList, { kind: 'package', pkg: 'shared', file: 'init.ts' })).toBe(true)
    expect(selectionExists(fixtureList, { kind: 'package', pkg: 'shared', file: 'missing.ts' })).toBe(false)
  })

  it('inserts directive once and creates valid starter source', () => {
    expect(insertPreludeDirective('const x = 1;', 2)).toBe('// af-prelude-version: 2\nconst x = 1;')
    expect(insertPreludeDirective('// af-prelude-version: 2\nconst x = 1;', 2)).toBe('// af-prelude-version: 2\nconst x = 1;')
    expect(packageStarterSource('shared', 2)).toContain('// af-prelude-version: 2')
  })
})
