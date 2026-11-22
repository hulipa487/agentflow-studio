import { describe, expect, it } from 'vitest'
import type { PluginsList } from '@/api/types'
import {
  LUA_FILE_NAME_RE,
  PACKAGE_NAME_RE,
  defaultSelection,
  insertPreludeDirective,
  normalizeLuaFilename,
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
  prelude_version: 1,
  builtins: [
    {
      name: 'token_budget',
      kind: 'support',
      summary: 'budget helper',
      shadowed: false,
      source: '-- af-prelude-version: 1\n',
      builtin_source: '-- af-prelude-version: 1\n',
    },
  ],
  packages: [
    {
      name: 'shared',
      dir: '/etc/agentflow/packages/shared',
      managed: true,
      files: [{ name: 'init.lua', path: '/etc/agentflow/packages/shared/init.lua', source: '-- af-prelude-version: 1\n' }],
    },
  ],
}

describe('plugins helpers', () => {
  it('validates package and file names', () => {
    expect(PACKAGE_NAME_RE.test('shared-helpers_v1')).toBe(true)
    expect(PACKAGE_NAME_RE.test('../bad')).toBe(false)
    expect(LUA_FILE_NAME_RE.test('helpers.lua')).toBe(true)
    expect(LUA_FILE_NAME_RE.test('helpers.txt')).toBe(false)
    expect(normalizeLuaFilename('util')).toBe('util.lua')
    expect(normalizeLuaFilename('util.lua')).toBe('util.lua')
  })

  it('tracks selection existence and defaults', () => {
    const def = defaultSelection(fixtureList)
    expect(selectionKey(def)).toBe('builtin:token_budget')
    expect(selectionExists(fixtureList, def)).toBe(true)
    expect(selectionExists(fixtureList, { kind: 'package', pkg: 'shared', file: 'init.lua' })).toBe(true)
    expect(selectionExists(fixtureList, { kind: 'package', pkg: 'shared', file: 'missing.lua' })).toBe(false)
  })

  it('inserts directive once and creates valid starter source', () => {
    expect(insertPreludeDirective('local x = 1', 1)).toBe('-- af-prelude-version: 1\nlocal x = 1')
    expect(insertPreludeDirective('-- af-prelude-version: 1\nlocal x = 1', 1)).toBe('-- af-prelude-version: 1\nlocal x = 1')
    expect(packageStarterSource('shared', 1)).toContain('-- af-prelude-version: 1')
  })
})
