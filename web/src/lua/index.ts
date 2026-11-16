// Luau language intelligence for the loop editor: the public surface the
// builder codes against. The analyzer behind it is a port of the desktop
// Studio's in-process language server (internal/lua): token.ts, lexer.ts,
// api.ts, analysis.ts and service.ts mirror the Go files of the same names,
// luau.ts adapts them to string offsets, and codemirror.ts is the editor wiring.

// The editor surface.
export { analyze, type LuauDiagnostic, type LuauOptions, type Severity } from './luau'
export { luauSupport } from './codemirror'

// Completion, hover and status outside the editor.
export {
  completionsAt,
  hoverAt,
  summarize,
  contextFor,
  type DiagnosticSummary,
  type LuauCompletions,
  type LuauHover,
} from './luau'
export { entryCard, hoverCard, luauExtension, luauSources, type LuauSources } from './codemirror'
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
