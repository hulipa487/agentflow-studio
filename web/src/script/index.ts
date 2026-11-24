// TypeScript/JS Prelude v2 language intelligence for loop, plugin, and safety editors.

export { analyze, type ScriptDiagnostic, type ScriptOptions, type Severity } from './offsets'
export { scriptSupport } from './codemirror'

export {
  completionsAt,
  hoverAt,
  summarize,
  contextFor,
  type DiagnosticSummary,
  type ScriptCompletions,
  type ScriptHover,
} from './offsets'
export { entryCard, hoverCard, scriptExtension, scriptSources, type ScriptSources } from './codemirror'
export { plainDoc, stripSnippet, type CompletionItem, type CompletionKind } from './service'

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

export { analyzeSource, type AnalysisContext, type AnalysisResult, type Local, type Model } from './analysis'
export { lex } from './lexer'
export type { Diagnostic, Position, Token } from './token'
