package lua

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"agentflow-studio/internal/i18n"
)

// Context is what the analyzer knows about the deployment and the agent.
type Context struct {
	// PreludeVersion is the runtime's prelude version. Zero disables the
	// version comparison (but not the directive requirement).
	PreludeVersion int
	// Capabilities is the agent's effective capability list. nil means
	// "unknown, do not check"; an empty non-nil list means no capabilities.
	Capabilities []string
}

// Local is a name declared inside the chunk, with the scope it lives in.
type Local struct {
	Name  string
	Kind  string // local, param, loop, self
	Start Position
	End   Position
	// Decl is the position of the declaring identifier.
	Decl Position
}

// Model is the reusable result of an analysis pass.
type Model struct {
	Source  string
	Tokens  []Token
	Locals  []*Local
	Globals map[string]bool
}

// Result is a full analysis of one document.
type Result struct {
	Diagnostics []Diagnostic
	Model       *Model
}

// Analyze lints one loop source.
func Analyze(src string, ctx Context) Result {
	tokens, diags := Lex(src)

	diags = append(diags, checkDirective(src, ctx.PreludeVersion)...)
	diags = append(diags, checkStructure(tokens)...)

	a := &analyzer{
		src:        src,
		tokens:     tokens,
		ctx:        ctx,
		skip:       map[int]bool{},
		hasImports: hasPackageImports(src),
		model:      &Model{Source: src, Tokens: tokens, Globals: map[string]bool{}},
	}
	a.run()
	diags = append(diags, a.diags...)
	diags = append(diags, a.entryPointDiagnostics()...)

	diags = dedupeDiagnostics(diags)
	sort.SliceStable(diags, func(i, j int) bool {
		if diags[i].Line != diags[j].Line {
			return diags[i].Line < diags[j].Line
		}
		if diags[i].Col != diags[j].Col {
			return diags[i].Col < diags[j].Col
		}
		return diags[i].Severity < diags[j].Severity
	})
	return Result{Diagnostics: diags, Model: a.model}
}

func dedupeDiagnostics(in []Diagnostic) []Diagnostic {
	seen := map[string]bool{}
	out := in[:0]
	for _, d := range in {
		key := fmt.Sprintf("%s|%d|%d|%s", d.Code, d.Line, d.Col, d.Message)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, d)
	}
	return out
}

// ---- directive -------------------------------------------------------------

var versionDirectiveRE = regexp.MustCompile(`^\s*--\s*af-prelude-version\s*:\s*(\d+)\s*$`)

// checkDirective mirrors the runtime's gate: the directive must appear exactly
// once in the leading run of blank and comment lines.
func checkDirective(src string, want int) []Diagnostic {
	type found struct {
		version int
		line    int
	}
	var hits []found
	for i, line := range strings.Split(src, "\n") {
		t := strings.TrimSpace(line)
		if t == "" {
			continue
		}
		if !strings.HasPrefix(t, "--") {
			break
		}
		if m := versionDirectiveRE.FindStringSubmatch(line); m != nil {
			n, _ := strconv.Atoi(m[1])
			hits = append(hits, found{version: n, line: i})
		}
	}
	if len(hits) == 0 {
		return []Diagnostic{{
			Line: 0, Col: 0, EndLine: 0, EndCol: 1,
			Severity: SeverityError, Code: "prelude.missing-directive",
			Message: i18n.T("lua.diag.missingDirective", "want", want),
		}}
	}
	var out []Diagnostic
	if len(hits) > 1 {
		for _, h := range hits[1:] {
			out = append(out, Diagnostic{
				Line: h.line, Col: 0, EndLine: h.line, EndCol: 1,
				Severity: SeverityError, Code: "prelude.duplicate-directive",
				Message: i18n.T("lua.diag.duplicateDirective"),
			})
		}
	}
	if want > 0 {
		for _, h := range hits {
			if h.version != want {
				out = append(out, Diagnostic{
					Line: h.line, Col: 0, EndLine: h.line, EndCol: 1,
					Severity: SeverityError, Code: "prelude.version-mismatch",
					Message: i18n.T("lua.diag.versionMismatch", "got", h.version, "want", want),
				})
			}
		}
	}
	return out
}

// ---- structure -------------------------------------------------------------

type blockFrame struct {
	kind string
	tok  Token
}

func checkStructure(tokens []Token) []Diagnostic {
	var diags []Diagnostic
	var blocks []blockFrame
	var brackets []struct {
		lit string
		tok Token
	}
	pendingIf := 0
	pendingLoop := ""

	report := func(t Token, code, msg string) {
		diags = append(diags, Diagnostic{
			Line: t.Line, Col: t.Col, EndLine: t.EndLine, EndCol: t.EndCol,
			Severity: SeverityError, Code: code, Message: msg,
		})
	}

	for _, t := range tokens {
		if t.Kind == TokenEOF {
			break
		}
		if t.Kind == TokenOp {
			switch t.Lit {
			case "(", "[", "{":
				brackets = append(brackets, struct {
					lit string
					tok Token
				}{t.Lit, t})
			case ")", "]", "}":
				want := map[string]string{")": "(", "]": "[", "}": "{"}[t.Lit]
				if len(brackets) == 0 {
					report(t, "syntax.unexpected-close", i18n.T("lua.diag.unexpectedClose", "token", strconv.Quote(t.Lit)))
				} else if top := brackets[len(brackets)-1]; top.lit != want {
					report(t, "syntax.mismatched-close", i18n.T("lua.diag.mismatchedClose", "token", strconv.Quote(t.Lit), "open", strconv.Quote(top.lit)))
					brackets = brackets[:len(brackets)-1]
				} else {
					brackets = brackets[:len(brackets)-1]
				}
			}
		}
		if t.Kind != TokenKeyword {
			continue
		}
		switch t.Lit {
		case "function":
			blocks = append(blocks, blockFrame{"function", t})
		case "if":
			pendingIf++
		case "then":
			if pendingIf > 0 {
				blocks = append(blocks, blockFrame{"if", t})
				pendingIf--
			}
		case "for":
			pendingLoop = "for"
		case "while":
			pendingLoop = "while"
		case "do":
			if pendingLoop != "" {
				blocks = append(blocks, blockFrame{pendingLoop, t})
				pendingLoop = ""
			} else {
				blocks = append(blocks, blockFrame{"do", t})
			}
		case "repeat":
			blocks = append(blocks, blockFrame{"repeat", t})
		case "elseif", "else":
			if len(blocks) == 0 || blocks[len(blocks)-1].kind != "if" {
				report(t, "syntax.unexpected-else", i18n.T("lua.diag.unexpectedElse", "token", strconv.Quote(t.Lit)))
			}
		case "until":
			if len(blocks) == 0 || blocks[len(blocks)-1].kind != "repeat" {
				report(t, "syntax.unexpected-until", i18n.T("lua.diag.unexpectedUntil"))
			} else {
				blocks = blocks[:len(blocks)-1]
			}
		case "end":
			if len(blocks) == 0 {
				report(t, "syntax.unexpected-end", i18n.T("lua.diag.unexpectedEnd"))
				continue
			}
			top := blocks[len(blocks)-1]
			if top.kind == "repeat" {
				report(t, "syntax.mismatched-end", i18n.T("lua.diag.mismatchedEnd"))
			}
			blocks = blocks[:len(blocks)-1]
		}
	}
	for _, b := range blocks {
		diags = append(diags, Diagnostic{
			Line: b.tok.Line, Col: b.tok.Col, EndLine: b.tok.EndLine, EndCol: b.tok.EndCol,
			Severity: SeverityError, Code: "syntax.unclosed-block",
			Message: i18n.T("lua.diag.unclosedBlock", "kind", b.kind),
		})
	}
	for _, b := range brackets {
		diags = append(diags, Diagnostic{
			Line: b.tok.Line, Col: b.tok.Col, EndLine: b.tok.EndLine, EndCol: b.tok.EndCol,
			Severity: SeverityError, Code: "syntax.unclosed-bracket",
			Message: i18n.T("lua.diag.unclosedBracket", "token", strconv.Quote(b.lit)),
		})
	}
	return diags
}

// ---- scope and API analysis ------------------------------------------------

type scopeFrame struct {
	kind     string // chunk, block, function, loop
	vars     map[string]*Local
	fn       bool
	loop     bool
	start    Position
	loopName string
}

type analyzer struct {
	src    string
	tokens []Token
	ctx    Context
	skip   map[int]bool
	model  *Model
	diags  []Diagnostic
	frames []*scopeFrame

	tableDepth int
	typeLine   int // identifiers on this line are type syntax
	loopLocal  bool

	pendingLoopNames []string
	pendingIf        int
	hasImports       bool
}

func (a *analyzer) run() {
	a.frames = []*scopeFrame{{kind: "chunk", vars: map[string]*Local{}, start: Position{}}}
	a.typeLine = -1

	for i := 0; i < len(a.tokens); i++ {
		t := a.tokens[i]
		if t.Kind == TokenEOF {
			break
		}
		if a.typeLine >= 0 && t.Line != a.typeLine {
			a.typeLine = -1
		}
		if a.typeLine >= 0 && t.Kind == TokenName {
			continue
		}
		if a.skip[i] {
			continue
		}
		if t.Is("{") {
			a.tableDepth++
		} else if t.Is("}") {
			if a.tableDepth > 0 {
				a.tableDepth--
			}
		}
		if t.Kind == TokenKeyword {
			switch t.Lit {
			case "function":
				frame, next := a.handleFunction(i, false)
				a.push(frame)
				i = next - 1
				continue
			case "local":
				i = a.handleLocal(i)
				continue
			case "for":
				a.pendingLoopNames, _ = a.parseForHeader(i)
				continue
			case "if":
				// The body starts at the first then; an elseif then continues
				// the same block rather than opening another one.
				a.pendingIf++
				continue
			case "then":
				if a.pendingIf > 0 {
					a.pendingIf--
					a.push(&scopeFrame{kind: "block", vars: map[string]*Local{}, start: posOf(t)})
				}
				continue
			case "do":
				kind := "block"
				vars := map[string]*Local{}
				if a.pendingLoopNames != nil {
					kind = "loop"
					for _, name := range a.pendingLoopNames {
						l := &Local{Name: name, Kind: "loop", Start: posOf(t), Decl: posOf(t)}
						vars[name] = l
						a.model.Locals = append(a.model.Locals, l)
					}
					a.pendingLoopNames = nil
				}
				a.push(&scopeFrame{kind: kind, vars: vars, start: posOf(t)})
				continue
			case "repeat":
				a.push(&scopeFrame{kind: "block", vars: map[string]*Local{}, start: posOf(t)})
				continue
			case "until":
				a.pop(posOf(t))
				if i+1 < len(a.tokens) {
					a.typeLine = -1
				}
				continue
			case "end":
				a.pop(posOf(t))
				continue
			case "return":
				if fn := a.nearestFunction(); fn != nil && fn.loop {
					a.diag(t, SeverityWarning, "api.return-loop",
						i18n.T("lua.diag.returnFromLoop"))
				}
				continue
			case "type":
				a.typeLine = t.Line
				continue
			}
			continue
		}
		if t.Kind == TokenName {
			i = a.handleName(i)
		}
	}
	a.finish()
}

// finish closes every open scope, then the chunk, so locals have an end
// position for completion.
func (a *analyzer) finish() {
	end := posOf(a.tokens[len(a.tokens)-1])
	for len(a.frames) > 1 {
		a.pop(end)
	}
	for _, l := range a.frames[0].vars {
		l.End = end
	}
}

func posOf(t Token) Position { return Position{t.Line, t.Col} }

func (a *analyzer) push(f *scopeFrame) {
	a.frames = append(a.frames, f)
}

func (a *analyzer) pop(at Position) {
	if len(a.frames) <= 1 {
		return
	}
	f := a.frames[len(a.frames)-1]
	a.frames = a.frames[:len(a.frames)-1]
	for _, l := range f.vars {
		l.End = at
	}
}

func (a *analyzer) nearestFunction() *scopeFrame {
	for i := len(a.frames) - 1; i >= 0; i-- {
		if a.frames[i].fn {
			return a.frames[i]
		}
	}
	return nil
}

func (a *analyzer) declare(t Token, kind string) *Local {
	l := &Local{Name: t.Lit, Kind: kind, Start: posOf(t), Decl: posOf(t)}
	top := a.frames[len(a.frames)-1]
	top.vars[t.Lit] = l
	a.model.Locals = append(a.model.Locals, l)
	return l
}

func (a *analyzer) resolveLocal(name string) bool {
	for i := len(a.frames) - 1; i >= 0; i-- {
		if _, ok := a.frames[i].vars[name]; ok {
			return true
		}
	}
	return false
}

func (a *analyzer) diag(t Token, sev Severity, code, msg string) {
	a.diags = append(a.diags, Diagnostic{
		Line: t.Line, Col: t.Col, EndLine: t.EndLine, EndCol: t.EndCol,
		Severity: sev, Code: code, Message: msg,
	})
}

// handleFunction parses a function header, declares its name and parameters,
// and returns the frame for its body plus the token index after the header.
func (a *analyzer) handleFunction(at int, local bool) (*scopeFrame, int) {
	t := a.tokens[at]
	j := at + 1
	name := ""
	simple := false
	dotted := false
	method := false
	var rootTok Token
	hasName := false

	if j < len(a.tokens) && a.tokens[j].Kind == TokenName {
		hasName = true
		name = a.tokens[j].Lit
		rootTok = a.tokens[j]
		j++
		for j+1 < len(a.tokens) && (a.tokens[j].Is(".") || a.tokens[j].Is(":")) && a.tokens[j+1].Kind == TokenName {
			if a.tokens[j].Is(":") {
				method = true
			}
			dotted = true
			j += 2
		}
		if j < len(a.tokens) && a.tokens[j].Is("<") {
			j = a.skipAngles(j)
		}
		simple = !dotted && j < len(a.tokens) && a.tokens[j].Is("(")
	}

	frame := &scopeFrame{kind: "function", vars: map[string]*Local{}, fn: true, start: posOf(t)}
	if hasName {
		if simple {
			if local {
				a.declare(rootTok, "function")
				if name == "loop" {
					a.loopLocal = true
				}
			} else {
				// Lua function statements create globals.
				a.model.Globals[name] = true
				if name == "loop" {
					frame.loop = true
					frame.loopName = name
				}
			}
		} else if dotted {
			// function a.b() assigns a field; a must resolve.
			a.checkRootUse(rootTok)
		}
	}
	if method {
		l := &Local{Name: "self", Kind: "self", Start: posOf(t), Decl: posOf(t)}
		frame.vars["self"] = l
		a.model.Locals = append(a.model.Locals, l)
	}
	if j < len(a.tokens) && a.tokens[j].Is("(") {
		j = a.parseParams(j, frame)
	}
	if j < len(a.tokens) && a.tokens[j].Is(":") {
		// Return type: identifiers on this line are types, not globals.
		a.typeLine = a.tokens[j].Line
		j++
	}
	return frame, j
}

func (a *analyzer) checkRootUse(t Token) {
	if a.resolveLocal(t.Lit) {
		return
	}
	if IsKnownGlobal(t.Lit) || a.model.Globals[t.Lit] {
		return
	}
	if msg, ok := unavailableGlobals[t.Lit]; ok {
		a.diag(t, SeverityError, "sandbox.unavailable", i18n.T(msg))
		return
	}
	if a.typeLine >= 0 {
		return
	}
	suggestion := Suggest(t.Lit, a.knownNames())
	msg := i18n.T("lua.diag.unknownGlobal", "name", strconv.Quote(t.Lit))
	if suggestion != "" {
		msg = i18n.T("lua.diag.unknownGlobalSuggest", "name", strconv.Quote(t.Lit), "suggestion", strconv.Quote(suggestion))
	}
	a.diag(t, SeverityWarning, "global.unknown", msg)
}

func (a *analyzer) parseParams(open int, frame *scopeFrame) int {
	depth := 0
	expect := true
	for j := open; j < len(a.tokens); j++ {
		t := a.tokens[j]
		if t.Is("(") {
			depth++
			continue
		}
		if t.Is(")") {
			depth--
			if depth == 0 {
				return j + 1
			}
			continue
		}
		if depth != 1 {
			continue
		}
		if t.Is(",") {
			expect = true
			continue
		}
		if t.Is(":") {
			j = a.skipTypeUntil(j+1, ",", ")") - 1
			continue
		}
		if expect && t.Kind == TokenName {
			l := &Local{Name: t.Lit, Kind: "param", Start: posOf(t), Decl: posOf(t)}
			frame.vars[t.Lit] = l
			a.model.Locals = append(a.model.Locals, l)
			expect = false
		}
	}
	return len(a.tokens)
}

// skipTypeUntil advances over a type expression, stopping before stop tokens at
// bracket depth zero. It returns the index of the stop token.
func (a *analyzer) skipTypeUntil(from int, stops ...string) int {
	depth := 0
	for j := from; j < len(a.tokens); j++ {
		t := a.tokens[j]
		if t.Kind == TokenEOF {
			return j
		}
		switch t.Lit {
		case "(", "{", "[", "<":
			depth++
			continue
		case ")", "}", "]", ">":
			if depth == 0 {
				return j
			}
			depth--
			continue
		}
		if depth == 0 {
			for _, s := range stops {
				if t.Is(s) {
					return j
				}
			}
		}
	}
	return len(a.tokens)
}

func (a *analyzer) skipAngles(at int) int {
	depth := 0
	for j := at; j < len(a.tokens); j++ {
		switch a.tokens[j].Lit {
		case "<":
			depth++
		case ">":
			depth--
			if depth == 0 {
				return j + 1
			}
		}
	}
	return at + 1
}

func (a *analyzer) handleLocal(at int) int {
	if at+1 < len(a.tokens) && a.tokens[at+1].Is("function") {
		frame, next := a.handleFunction(at+1, true)
		a.push(frame)
		return next - 1
	}

	j := at + 1
	expect := true
	for j < len(a.tokens) {
		t := a.tokens[j]
		if t.Kind == TokenEOF {
			break
		}
		if t.Is(",") {
			expect = true
			j++
			continue
		}
		if t.Is("=") {
			break
		}
		if t.Is(":") {
			j = a.skipTypeUntil(j+1, ",", "=")
			expect = false
			continue
		}
		if expect && t.Kind == TokenName {
			a.declare(t, "local")
			expect = false
			j++
			continue
		}
		break
	}
	return j - 1
}

// parseForHeader collects loop variable names and the index of the closing do.
func (a *analyzer) parseForHeader(at int) ([]string, int) {
	var names []string
	expect := true
	depth := 0
	mode := "names"
	doAt := -1
	for j := at + 1; j < len(a.tokens); j++ {
		t := a.tokens[j]
		if t.Kind == TokenEOF {
			break
		}
		switch {
		case t.Is("(") || t.Is("{") || t.Is("["):
			depth++
			continue
		case t.Is(")") || t.Is("}") || t.Is("]"):
			if depth > 0 {
				depth--
			}
			continue
		}
		if depth != 0 {
			continue
		}
		if t.Is("do") {
			doAt = j
			break
		}
		if t.Is(",") && mode == "names" {
			expect = true
			continue
		}
		if t.Is("=") || t.Is("in") {
			mode = "expr"
			continue
		}
		if mode == "names" && expect && t.Kind == TokenName {
			names = append(names, t.Lit)
			a.skip[j] = true
			expect = false
		}
	}
	if doAt < 0 {
		return names, len(a.tokens) - 1
	}
	// A generic for over llm.stream is refused by the bridge.
	for j := at + 1; j < doAt; j++ {
		if a.tokens[j].Is("llm") && j+2 < len(a.tokens) && a.tokens[j+1].Is(".") && a.tokens[j+2].Is("stream") {
			a.diag(a.tokens[j], SeverityError, "api.stream-for",
				i18n.T("lua.diag.streamFor"))
			break
		}
	}
	return names, doAt
}

func (a *analyzer) handleName(at int) int {
	t := a.tokens[at]
	if at > 0 && (a.tokens[at-1].Is(".") || a.tokens[at-1].Is(":")) {
		return at
	}
	// Table constructor key: { name = ... }
	if a.tableDepth > 0 && at+1 < len(a.tokens) && a.tokens[at+1].Is("=") &&
		(at == 0 || a.tokens[at-1].Is("{") || a.tokens[at-1].Is(",") || a.tokens[at-1].Is(";")) {
		return at
	}

	// Build the dotted path so field names are not treated as globals.
	parts := []string{t.Lit}
	j := at
	for j+2 < len(a.tokens) && (a.tokens[j+1].Is(".") || a.tokens[j+1].Is(":")) && a.tokens[j+2].Kind == TokenName {
		parts = append(parts, a.tokens[j+2].Lit)
		j += 2
	}

	if a.resolveLocal(t.Lit) {
		return j
	}

	if msg, bad := unavailableGlobals[t.Lit]; bad && !IsKnownGlobal(t.Lit) && !a.model.Globals[t.Lit] {
		a.diag(t, SeverityError, "sandbox.unavailable", i18n.T(msg))
		return j
	}

	if !IsKnownGlobal(t.Lit) && !a.model.Globals[t.Lit] {
		if a.typeLine >= 0 {
			return j
		}
		if at+1 < len(a.tokens) && a.tokens[at+1].Is("=") {
			suggestion := Suggest(t.Lit, a.knownNames())
			msg := i18n.T("lua.diag.undeclaredAssignment", "name", strconv.Quote(t.Lit))
			if suggestion != "" {
				msg = i18n.T("lua.diag.undeclaredAssignmentSuggest", "name", strconv.Quote(t.Lit), "suggestion", strconv.Quote(suggestion))
			}
			a.diag(t, SeverityWarning, "global.assignment", msg)
			return j
		}
		if a.hasImports {
			// A package import contributes source the analyzer cannot see; its
			// globals would otherwise all be reported as unknown.
			return j
		}
		suggestion := Suggest(t.Lit, a.knownNames())
		msg := i18n.T("lua.diag.unknownGlobal", "name", strconv.Quote(t.Lit))
		if suggestion != "" {
			msg = i18n.T("lua.diag.unknownGlobalSuggest", "name", strconv.Quote(t.Lit), "suggestion", strconv.Quote(suggestion))
		}
		a.diag(t, SeverityWarning, "global.unknown", msg)
		return j
	}

	// Validate the field chain against the documented prelude. Standard library
	// tables (string, table, math, ...) are deliberately not enumerated: the
	// analyzer only knows the AgentFlow surface.
	if len(parts) > 1 && apiGlobals[parts[0]] != nil {
		ok := true
		for k := 1; k < len(parts); k++ {
			prefix := strings.Join(parts[:k+1], ".")
			if Lookup(prefix) != nil {
				continue
			}
			parent := strings.Join(parts[:k], ".")
			fields := FieldNames(parent)
			segTok := a.tokens[at+2*k]
			msg := i18n.T("lua.diag.unknownField", "field", strconv.Quote(parts[k]), "parent", strconv.Quote(parent))
			if s := Suggest(parts[k], fields); s != "" {
				msg = i18n.T("lua.diag.unknownFieldSuggest", "field", strconv.Quote(parts[k]), "parent", strconv.Quote(parent), "suggestion", strconv.Quote(s))
			} else if len(fields) == 0 {
				msg = i18n.T("lua.diag.noPublicFields", "parent", strconv.Quote(parent))
			}
			a.diag(segTok, SeverityWarning, "field.unknown", msg)
			ok = false
			break
		}
		if !ok {
			return j
		}
	}

	path := strings.Join(parts, ".")
	entry := Lookup(path)
	if entry == nil {
		return j
	}
	if path == "af.op" {
		a.diag(t, SeverityWarning, "api.af-op",
			i18n.T("lua.diag.afOp"))
	}
	// A call is a path immediately followed by "(".
	isCall := j+1 < len(a.tokens) && a.tokens[j+1].Is("(")
	if !isCall {
		return j
	}
	if entry.Maintenance {
		a.diag(t, SeverityWarning, "api.maintenance-only",
			i18n.T("lua.diag.maintenanceOnly", "path", path))
	}
	if entry.Capability != "" && a.ctx.Capabilities != nil {
		if !contains(a.ctx.Capabilities, entry.Capability) {
			a.diag(t, SeverityWarning, "capability.missing",
				i18n.T("lua.diag.capabilityMissing", "path", path, "capability", strconv.Quote(entry.Capability)))
		}
	}
	if entry.Yields {
		if fn := a.nearestFunction(); fn == nil {
			a.diag(t, SeverityWarning, "api.top-level-yield",
				i18n.T("lua.diag.topLevelYield", "path", path))
		}
	}
	return j
}

func (a *analyzer) knownNames() []string {
	seen := map[string]bool{}
	var out []string
	add := func(n string) {
		if n != "" && !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
	}
	for n := range apiGlobals {
		add(n)
	}
	for n := range stdlibGlobals {
		add(n)
	}
	for n := range a.model.Globals {
		add(n)
	}
	for _, l := range a.model.Locals {
		add(l.Name)
	}
	return out
}

func contains(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}

func (a *analyzer) entryPointDiagnostics() []Diagnostic {
	if a.model.Globals["loop"] {
		return nil
	}
	if a.loopLocal {
		return []Diagnostic{{
			Line: 0, Col: 0, EndLine: 0, EndCol: 1,
			Severity: SeverityError, Code: "entry.local-loop",
			Message: i18n.T("lua.diag.localLoop"),
		}}
	}
	line := len(strings.Split(a.src, "\n")) - 1
	return []Diagnostic{{
		Line: line, Col: 0, EndLine: line, EndCol: 1,
		Severity: SeverityError, Code: "entry.missing-loop",
		Message: i18n.T("lua.diag.missingLoop"),
	}}
}

// hasPackageImports reports whether the chunk imports a deployment package.
// Package exports are concatenated before the chunk at boot, so names they
// define are invisible to this analyzer; unknown-global warnings are skipped
// for such chunks rather than reporting every package call as a typo.
func hasPackageImports(src string) bool {
	for _, line := range strings.Split(src, "\n") {
		t := strings.TrimSpace(line)
		if t == "" || strings.HasPrefix(t, "--") {
			if strings.HasPrefix(strings.TrimSpace(strings.TrimPrefix(t, "--")), "af-import:") {
				return true
			}
			continue
		}
		break
	}
	return false
}
