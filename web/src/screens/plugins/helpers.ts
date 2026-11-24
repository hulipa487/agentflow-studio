import type { BuiltinPlugin, PackageFile, PluginsList, ScriptPackage } from '@/api/types'

export type Selection =
  | { kind: 'builtin'; name: string }
  | { kind: 'package'; pkg: string; file: string }

export const PACKAGE_NAME_RE = /^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$/
export const SCRIPT_FILE_NAME_RE = /^[A-Za-z0-9][A-Za-z0-9._-]{0,63}\.(ts|js)$/

export function selectionKey(sel: Selection | null): string {
  if (!sel) return ''
  return sel.kind === 'builtin' ? `builtin:${sel.name}` : `pkg:${sel.pkg}/${sel.file}`
}

export function sameSelection(a: Selection | null, b: Selection | null): boolean {
  return selectionKey(a) === selectionKey(b)
}

export function selectionExists(list: PluginsList, sel: Selection | null): boolean {
  if (!sel) return false
  if (sel.kind === 'builtin') {
    return (list.builtins ?? []).some((b) => b.name === sel.name)
  }
  const pkg = (list.packages ?? []).find((p) => p.name === sel.pkg)
  return Boolean(pkg?.files?.some((f) => f.name === sel.file))
}

export function defaultSelection(list: PluginsList): Selection | null {
  const firstBuiltin = list.builtins?.[0]
  if (firstBuiltin) return { kind: 'builtin', name: firstBuiltin.name }
  const firstPkg = list.packages?.[0]
  const firstFile = firstPkg?.files?.[0]
  if (firstPkg && firstFile) return { kind: 'package', pkg: firstPkg.name, file: firstFile.name }
  return null
}

export function findSelectedBuiltin(list: PluginsList | undefined, sel: Selection | null): BuiltinPlugin | null {
  if (!list || !sel || sel.kind !== 'builtin') return null
  return (list.builtins ?? []).find((b) => b.name === sel.name) ?? null
}

export function findSelectedPackageFile(
  list: PluginsList | undefined,
  sel: Selection | null,
): { pkg: ScriptPackage; file: PackageFile } | null {
  if (!list || !sel || sel.kind !== 'package') return null
  const pkg = (list.packages ?? []).find((p) => p.name === sel.pkg)
  if (!pkg) return null
  const file = (pkg.files ?? []).find((f) => f.name === sel.file)
  if (!file) return null
  return { pkg, file }
}

export function isValidScriptFilename(name: string): boolean {
  return SCRIPT_FILE_NAME_RE.test(name) && !name.endsWith('.d.ts')
}

export function normalizeScriptFilename(raw: string): string {
  const trimmed = raw.trim()
  if (!trimmed) return ''
  return trimmed.endsWith('.ts') || trimmed.endsWith('.js') ? trimmed : `${trimmed}.ts`
}

export function insertPreludeDirective(source: string, version: number): string {
  const directive = `// af-prelude-version: ${version > 0 ? version : 2}`
  if (source.split('\n').some((l) => l.trim().startsWith('// af-prelude-version:'))) return source
  return source === '' ? `${directive}\n` : `${directive}\n${source}`
}

export function packageStarterSource(pkgName: string, version: number): string {
  const v = version > 0 ? version : 2
  return `// af-prelude-version: ${v}\n// Shared package: ${pkgName}\n\nvar ${pkgName.replace(/[^A-Za-z0-9_]/g, '_')} = {};\n`
}
