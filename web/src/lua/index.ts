// Lua 5.1 language intelligence for the loop editor: the public surface the
// builder codes against. The analyzer behind it mirrors the runtime's Lua
// analyzer (token.ts, lexer.ts, api.ts, analysis.ts, service.ts), offsets.ts
// adapts them to UTF-16 string offsets, and codemirror.ts is the editor wiring.

// The editor surface.
export { analyze, type LuaDiagnostic, type LuaOptions, type Severity } from './offsets'
export { luaSupport } from './codemirror'

// Completion, hover and status outside the editor.
export {
  completionsAt,
  hoverAt,
  summarize,
  contextFor,
  type DiagnosticSummary,
  type LuaCompletions,
  type LuaHover,
} from './offsets'
export { entryCard, hoverCard, luaExtension, luaSources, type LuaSources } from './codemirror'
export { plainDoc, stripSnippet, type CompletionItem, type CompletionKind } from './service'

// The prelude API catalog.
export {
  preludeEntries,
  lookup,
  children,
  fieldNames,
  isGlobalName,
  isKnownGlobal,
  stdlibGlobals,
  unavailableGlobals,
  type ApiEntry,
  type SymbolKind,
} from './api'

// The analyzer itself, in the Go original's terms (line/column positions,
// numeric severities, a tri-state capability list).
export { analyzeSource, type AnalysisContext, type AnalysisResult, type Local, type Model } from './analysis'
export { lex } from './lexer'
export type { Diagnostic, Position, Token } from './token'
