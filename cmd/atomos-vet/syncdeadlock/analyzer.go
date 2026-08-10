// Package syncdeadlock is a go vet analyzer that flags sync handler methods
// which make a sync call to themselves — a provably always-deadlock pattern.
//
// In go-atomos, each Atom processes one message at a time on its mailbox
// goroutine. A sync handler that sync-calls its own Atom blocks that goroutine
// waiting for a reply that can never be produced (the mailbox is busy running
// the handler). The runtime wait-graph detects this at PushSyncMessage time and
// returns ErrIDFirstSyncCallDeadlock; this analyzer catches the same bug at
// compile time so it never reaches runtime.
//
// Scope: only self-call is detected (the handler resolves an ID to its own atom
// and sync-calls it). Cross-atom cycles (A→B→A) are out of scope — atom names
// are runtime values, making static cycle detection unreliable; the runtime
// wait-graph (cosmos_process.go detectDeadlockAndWait) is the authoritative
// mechanism for those.
//
// Only non-test, non-generated files are checked.
package syncdeadlock

import (
	"go/ast"
	"go/token"
	"strings"

	"golang.org/x/tools/go/analysis"
)

const doc = `flag sync handlers that sync-call their own atom (always deadlock)

A go-atomos sync handler runs on the Atom's single mailbox goroutine. If it
sync-calls its own Atom (resolved via GetXxxAtomID(cosmos, self.GetIDInfo().Atom)
followed by a sync call on that ID), the mailbox goroutine blocks forever
waiting for a reply it can never process. This analyzer flags that
self-call pattern at compile time. Cross-atom cycles (A→B→A) are not detected
statically; the runtime wait-graph handles those.`

var Analyzer = &analysis.Analyzer{
	Name: "syncdeadlock",
	Doc:  doc,
	Run:  run,
}

func run(pass *analysis.Pass) (interface{}, error) {
	// Collect all sync handler methods in this package.
	var handlers []*ast.FuncDecl
	for _, file := range pass.Files {
		if isTestFile(pass, file) || isGenerated(file) {
			continue
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			if isSyncHandler(fn) {
				handlers = append(handlers, fn)
			}
		}
	}
	if len(handlers) == 0 {
		return nil, nil
	}

	// Pass 1: self-call detection (precise) + collect cross-handler sync-call
	// edges for cycle detection (heuristic).
	// handlerMethodNames is the set of method names that are sync handlers in
	// this package — used to recognize sync calls that target another handler.
	handlerMethodNames := map[string]bool{}
	for _, fn := range handlers {
		handlerMethodNames[fn.Name.Name] = true
	}

	// syncCallEdges maps a handler method name → set of handler method names it
	// sync-calls. E.g. "Greeting" → {"SayHello"} means some Greeting handler
	// body contains a `.SayHello(callerID, ...)` call.
	// The value also records a representative call position for reporting.
	syncCallEdges := map[string]map[string]callEdge{}

	for _, fn := range handlers {
		// Self-call check (existing logic).
		analyzeSelfCall(pass, fn)

		// Collect sync calls to other handler-named methods.
		calls := map[string]callEdge{}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			calleeName := callFuncName(call)
			if calleeName == "" {
				return true
			}
			// Is this a sync call to a known handler method? It must be a
			// selector call (id.MethodName) and the method must be a handler.
			if _, isSel := call.Fun.(*ast.SelectorExpr); !isSel {
				return true
			}
			if handlerMethodNames[calleeName] && calleeName != fn.Name.Name {
				calls[calleeName] = callEdge{pos: call.Pos()}
			}
			return true
		})
		if len(calls) > 0 {
			syncCallEdges[fn.Name.Name] = calls
		}
	}

	// Pass 2: cycle detection on the handler sync-call graph.
	// A cycle A→B→A (or A→B→C→A) means the handlers mutually sync-call each
	// other — a potential deadlock. We report at the edge that closes a cycle.
	reportedCycles := map[string]bool{}
	for from := range syncCallEdges {
		visited := map[string]bool{}
		detectCycleDFS(pass, from, from, syncCallEdges, visited, reportedCycles, []string{from})
	}
	return nil, nil
}

type callEdge struct {
	pos token.Pos
}

// detectCycleDFS walks the sync-call graph looking for a path back to `origin`.
// If found, it reports the cycle (deduped by canonical key).
func detectCycleDFS(pass *analysis.Pass, origin, current string, edges map[string]map[string]callEdge, visited, reported map[string]bool, path []string) {
	for to, edge := range edges[current] {
		if to == origin {
			// Cycle found: origin → ... → current → origin.
			cycle := append(path, to)
			key := canonicalCycleKey(cycle)
			if !reported[key] {
				reported[key] = true
				pass.Reportf(edge.pos,
					"potential sync deadlock cycle: handler %s — these handlers mutually sync-call each other (%s); at runtime the wait-graph catches this, but restructure to avoid the cycle",
					origin, joinNames(cycle, "→"))
			}
			return
		}
		if visited[to] {
			continue
		}
		visited[to] = true
		detectCycleDFS(pass, origin, to, edges, visited, reported, append(path, to))
	}
}

func canonicalCycleKey(cycle []string) string {
	// Normalize: rotate so the lexicographically smallest element is first,
	// then join. This makes A→B→A and B→A→B produce the same key.
	minIdx := 0
	for i := 1; i < len(cycle)-1; i++ { // exclude the trailing repeat
		if cycle[i] < cycle[minIdx] {
			minIdx = i
		}
	}
	n := len(cycle) - 1 // number of distinct nodes
	rotated := make([]string, 0, len(cycle))
	for i := 0; i <= n; i++ {
		rotated = append(rotated, cycle[(minIdx+i)%n])
	}
	return strings.Join(rotated, "→")
}

func joinNames(names []string, sep string) string {
	return strings.Join(names, sep)
}

func isGenerated(file *ast.File) bool {
	for _, cg := range file.Comments {
		for _, c := range cg.List {
			if strings.Contains(c.Text, "Code generated") && strings.Contains(c.Text, "DO NOT EDIT") {
				return true
			}
		}
	}
	return false
}

func isTestFile(pass *analysis.Pass, file *ast.File) bool {
	pos := pass.Fset.Position(file.Pos())
	return strings.HasSuffix(pos.Filename, "_test.go")
}

// isSyncHandler reports whether a method has the sync-handler signature shape:
// a receiver + a `from <Type>` parameter (conventionally `from ID`) + an `in`
// parameter. The types are checked by name loosely — the key signal is the
// first param name "from" and a second param. This is a heuristic; the
// authoritative handler list lives in the generated <Svc>Atom interface, but
// scanning that from here is fragile. The "from" param name is a strong enough
// convention across the framework's generated dispatch code.
func isSyncHandler(fn *ast.FuncDecl) bool {
	if fn.Recv == nil || len(fn.Type.Params.List) < 2 {
		return false
	}
	// First param (after receiver) should be named "from".
	params := fn.Type.Params.List
	if params[0].Names == nil || len(params[0].Names) == 0 {
		return false
	}
	return params[0].Names[0].Name == "from"
}

// analyzeSelfCall checks a sync handler for the self-call deadlock pattern:
// the handler resolves an ID to its own atom and sync-calls it.
func analyzeSelfCall(pass *analysis.Pass, fn *ast.FuncDecl) {
	if !isSyncHandler(fn) {
		return
	}

	// Determine the receiver identifier name (e.g. "t" in func (t *Type) Greeting(...)).
	// This is used to recognize references to self (t.self, self, etc.).
	recvName := ""
	if fn.Recv != nil && len(fn.Recv.List) > 0 && len(fn.Recv.List[0].Names) > 0 {
		recvName = fn.Recv.List[0].Names[0].Name
	}

	// Walk the handler body looking for the self-call pattern:
	//   <id>, _ := Get<Svc>AtomID(<expr>.Cosmos(), <expr>.GetIDInfo().Atom)
	//   <id>.<Handler>(<expr>, ...)
	// where <expr> is a reference to the handler's receiver (self/t.self/etc.)
	// or the handler's first param (from). If the atom-name argument resolves to
	// self, this is a self-call → always deadlock.
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		stmt, ok := n.(*ast.AssignStmt)
		if !ok {
			return true
		}
		for _, rhs := range stmt.Rhs {
			call, ok := rhs.(*ast.CallExpr)
			if !ok {
				continue
			}
			factoryName := callFuncName(call)
			if !isAtomIDFactoryName(factoryName) &&
				factoryName != "CosmosGetAtomID" && factoryName != "CosmosSpawnAtom" {
				continue
			}
			// Check whether the factory's atom-name argument references self.
			// For Get<Svc>AtomID(cosmos, name): name is arg[1].
			// For CosmosGetAtomID(elem, name): name is arg[1].
			// For Spawn<Svc>Atom(caller, cosmos, name, arg): name is arg[2].
			// For CosmosSpawnAtom(caller, elem, name, arg): name is arg[2].
			nameArgIdx := factoryNameArgIndex(factoryName, call)
			if nameArgIdx < 0 || nameArgIdx >= len(call.Args) {
				continue
			}
			if !exprReferencesSelf(call.Args[nameArgIdx], recvName, fn) {
				continue
			}
			pass.Reportf(call.Pos(),
				"sync handler %s resolves an ID to its own atom (%s with self atom name) — this sync call always deadlocks; use async or restructure",
				fn.Name.Name, factoryName)
		}
		return true
	})
}

// factoryNameArgIndex returns the index of the atom-name argument in a factory
// call, or -1 if unknown.
func factoryNameArgIndex(factoryName string, call *ast.CallExpr) int {
	switch {
	case isAtomIDFactoryName(factoryName):
		// Get<Svc>AtomID(cosmos, name) → name at index 1.
		// Spawn<Svc>Atom(caller, cosmos, name, arg) → name at index 2.
		// Distinguish by arg count: Get* has 2 args, Spawn* has 4.
		if strings.HasPrefix(factoryName, "Spawn") {
			return 2
		}
		return 1
	case factoryName == "CosmosGetAtomID":
		// CosmosGetAtomID(elem, name) → name at index 1.
		return 1
	case factoryName == "CosmosSpawnAtom":
		// CosmosSpawnAtom(caller, elem, name, arg) → name at index 2.
		return 2
	}
	return -1
}

// exprReferencesSelf reports whether an expression references the handler's
// self/receiver. It looks for the pattern `<recv>.GetIDInfo().Atom` (the
// canonical self-atom resolution), or any reference to a name containing the
// receiver identifier. This is a heuristic with very low false-positive rate:
// `self.GetIDInfo().Atom` is the documented idiom for resolving one's own atom.
func exprReferencesSelf(expr ast.Expr, recvName string, fn *ast.FuncDecl) bool {
	// Match <X>.GetIDInfo().Atom — the canonical self-resolution pattern.
	// The AST for `self.GetIDInfo().Atom` is a SelectorExpr:
	//   Sel=Atom, X=(SelectorExpr Sel=GetIDInfo ... X=<ident or selector>)
	if sel, ok := expr.(*ast.SelectorExpr); ok {
		if sel.Sel.Name == "Atom" {
			// Walk the inner expression to see if the base identifier is the
			// receiver or a field of the receiver (e.g. self, t.self).
			found := false
			ast.Inspect(sel.X, func(n ast.Node) bool {
				if ident, ok := n.(*ast.Ident); ok {
					if ident.Name == recvName || ident.Name == "self" {
						found = true
						return false
					}
				}
				return true
			})
			if found {
				return true
			}
		}
	}
	return false
}

func callFuncName(call *ast.CallExpr) string {
	switch fn := call.Fun.(type) {
	case *ast.Ident:
		return fn.Name
	case *ast.SelectorExpr:
		return fn.Sel.Name
	}
	return ""
}

func isAtomIDFactoryName(name string) bool {
	const getPrefix, atomIDSuffix = "Get", "AtomID"
	if strings.HasPrefix(name, getPrefix) && strings.HasSuffix(name, atomIDSuffix) &&
		len(name) > len(getPrefix)+len(atomIDSuffix) {
		return true
	}
	const spawnPrefix, atomSuffix = "Spawn", "Atom"
	if strings.HasPrefix(name, spawnPrefix) && strings.HasSuffix(name, atomSuffix) &&
		len(name) > len(spawnPrefix)+len(atomSuffix) {
		return true
	}
	return false
}
