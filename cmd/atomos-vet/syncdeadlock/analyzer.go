// Package syncdeadlock is a go vet analyzer that flags sync handler methods
// which make a sync call to themselves — a provably always-deadlock pattern —
// and handler call graphs that form a sync cycle.
//
// In go-atomos, each Atom processes one message at a time on its mailbox
// goroutine. A sync handler that sync-calls its own Atom blocks that goroutine
// waiting for a reply that can never be produced (the mailbox is busy running
// the handler). The runtime wait-graph detects this at PushSyncMessage time and
// returns ErrIDFirstSyncCallDeadlock; this analyzer catches the same bug at
// compile time so it never reaches runtime.
//
// Detection is type-based, keyed off the shapes emitted by protoc-gen-go-atomos:
//
//   - A handler interface is a named interface ending in "Atom" or "Element"
//     that embeds an interface named "Atomos" (e.g. GreeterAtom,
//     GreeterElement). Its explicit methods are the sync handlers.
//   - A named type in this package implementing such an interface (checked via
//     go/types Implements) provides those handlers.
//   - A sync call edge is a method call whose receiver's type is an ID client
//     type (named type ending in "AtomID"/"ElementID"), or a call to a
//     generated SpawnXxxAtom package function, or SyncMessagingByName /
//     CosmosSpawnAtom with a literal message/element name. Async variants
//     (AsyncXxx methods, callback-taking signatures) are excluded.
//
// Cycle nodes are (service, Element/Atom kind, method) triples, so same-named
// methods on unrelated services or on an Element versus an Atom cannot create
// false edges. Calls lexically inside a `go` statement are ignored: they run on
// another goroutine and do not block the mailbox.
//
// Scope limits: handler bodies in OTHER packages are not analyzed (an edge
// leaving this package has no returning edges here, so cross-package cycles
// are one-sided and go unreported); calls with runtime-computed message names
// are skipped; self-call detection still relies on the documented
// self.GetIDInfo().Atom resolution pattern. The runtime wait-graph
// (cosmos_process.go detectDeadlockAndWait) remains the authoritative
// mechanism for everything static analysis cannot see.
//
// Only non-test, non-generated files are checked.
package syncdeadlock

import (
	"go/ast"
	"go/token"
	"go/types"
	"sort"
	"strings"

	"golang.org/x/tools/go/analysis"
)

const doc = `flag sync handlers that sync-call their own atom, or handler call cycles

A go-atomos sync handler runs on the Atom's single mailbox goroutine. If it
sync-calls its own Atom (resolved via GetXxxAtomID(cosmos, self.GetIDInfo().Atom)
followed by a sync call on that ID), the mailbox goroutine blocks forever
waiting for a reply it can never process. This analyzer flags that self-call
pattern, plus cycles in the typed sync-call graph between handlers
(A.Svc.M → B.Svc.N → A.Svc.M), at compile time. Cross-package cycles are
one-sided and not reported; the runtime wait-graph handles those.`

var Analyzer = &analysis.Analyzer{
	Name: "syncdeadlock",
	Doc:  doc,
	Run:  run,
}

// serviceMethod identifies one sync handler. Element and Atom handlers remain
// distinct even when their service and method names are identical.
type serviceMethod struct {
	service string
	kind    string
	method  string
}

func (s serviceMethod) String() string { return s.service + "." + s.kind + "." + s.method }

// handlerDecl pairs a handler node with its implementing method declaration.
type handlerDecl struct {
	node     serviceMethod
	typeName string
	fn       *ast.FuncDecl
}

// handlerIface is a generated handler interface discovered in this package or
// one of its direct imports.
type handlerIface struct {
	service string
	kind    string
	iface   *types.Interface
	methods []string // explicit method names
}

func run(pass *analysis.Pass) (any, error) {
	// Index method declarations by (receiver base type, method name) so
	// implementing types can be linked back to AST bodies.
	methodDecls := map[string]map[string]*ast.FuncDecl{}
	for _, file := range pass.Files {
		if isTestFile(pass, file) || isGenerated(file) {
			continue
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || fn.Recv == nil {
				continue
			}
			recv := receiverTypeName(fn)
			if recv == "" {
				continue
			}
			if methodDecls[recv] == nil {
				methodDecls[recv] = map[string]*ast.FuncDecl{}
			}
			methodDecls[recv][fn.Name.Name] = fn
		}
	}
	if len(methodDecls) == 0 {
		return nil, nil
	}

	// Discover handler interfaces and the implementing types in this package.
	handlerIfaces := findHandlerInterfaces(pass)
	var handlers []handlerDecl
	if len(handlerIfaces) > 0 {
		for _, name := range pass.Pkg.Scope().Names() {
			tn, ok := pass.Pkg.Scope().Lookup(name).(*types.TypeName)
			if !ok || tn.IsAlias() {
				continue
			}
			named, ok := tn.Type().(*types.Named)
			if !ok || named.TypeParams() != nil {
				continue
			}
			for _, hi := range handlerIfaces {
				if !implementsHandler(named, hi.iface) {
					continue
				}
				for _, m := range hi.methods {
					if fn, ok := methodDecls[name][m]; ok {
						handlers = append(handlers, handlerDecl{
							node:     serviceMethod{service: hi.service, kind: hi.kind, method: m},
							typeName: name,
							fn:       fn,
						})
					}
				}
			}
		}
	}
	if len(handlers) == 0 {
		return nil, nil
	}

	// Pass 1: self-call detection (precise) + collect typed sync-call edges.
	// syncCallEdges maps handler → set of handlers it sync-calls, with a
	// representative call position for reporting.
	syncCallEdges := map[serviceMethod]map[serviceMethod]callEdge{}
	knownHandlers := map[serviceMethod]bool{}
	for _, h := range handlers {
		knownHandlers[h.node] = true
	}
	for _, h := range handlers {
		analyzeSelfCall(pass, h, knownHandlers)
		collectSyncCallEdges(pass, h, syncCallEdges)
	}

	// Pass 2: cycle detection on the handler sync-call graph.
	// A cycle A→B→A (or A→B→C→A) means the handlers mutually sync-call each
	// other — a potential deadlock. Cycles are collected first, then reported
	// deterministically: each unique cycle is rotated so its smallest node is
	// first and reported at the edge closing that rotation.
	cycles := map[string][]serviceMethod{}
	for _, from := range sortedNodes(syncCallEdges) {
		visited := map[serviceMethod]bool{}
		collectCycleDFS(from, from, syncCallEdges, visited, cycles, []serviceMethod{from})
	}
	for _, key := range sortedCycleKeys(cycles) {
		cycle := cycles[key]
		closing := cycle[len(cycle)-2]
		pass.Reportf(syncCallEdges[closing][cycle[len(cycle)-1]].pos,
			"potential sync deadlock cycle: handler %s — these handlers mutually sync-call each other (%s); at runtime the wait-graph catches this, but restructure to avoid the cycle",
			cycle[0], joinNames(cycle, "→"))
	}
	return nil, nil
}

// collectCycleDFS walks the sync-call graph looking for a path back to
// `origin`. If found, the cycle is stored (deduped by canonical key, rotated so
// the smallest node is first and repeated last).
func collectCycleDFS(origin, current serviceMethod, edges map[serviceMethod]map[serviceMethod]callEdge, visited map[serviceMethod]bool, cycles map[string][]serviceMethod, path []serviceMethod) {
	for _, to := range sortedNeighbors(edges[current]) {
		if to == origin {
			// Cycle found: origin → ... → current → origin.
			cycle := append(path, to)
			key := canonicalCycleKey(cycle)
			if _, seen := cycles[key]; !seen {
				cycles[key] = rotateCycle(cycle)
			}
			return
		}
		if visited[to] {
			continue
		}
		visited[to] = true
		collectCycleDFS(origin, to, edges, visited, cycles, append(path, to))
	}
}

// rotateCycle rotates a cycle (whose last element repeats the first) so the
// lexicographically smallest node is first — canonical form for dedup and
// deterministic reporting.
func rotateCycle(cycle []serviceMethod) []serviceMethod {
	minIdx := 0
	for i := 1; i < len(cycle)-1; i++ { // exclude the trailing repeat
		if cycle[i].String() < cycle[minIdx].String() {
			minIdx = i
		}
	}
	n := len(cycle) - 1 // number of distinct nodes
	rotated := make([]serviceMethod, 0, len(cycle))
	for i := 0; i <= n; i++ {
		rotated = append(rotated, cycle[(minIdx+i)%n])
	}
	return rotated
}

func sortedNodes(edges map[serviceMethod]map[serviceMethod]callEdge) []serviceMethod {
	out := make([]serviceMethod, 0, len(edges))
	for from := range edges {
		out = append(out, from)
	}
	sortSlice(out)
	return out
}

func sortedNeighbors(edges map[serviceMethod]callEdge) []serviceMethod {
	out := make([]serviceMethod, 0, len(edges))
	for to := range edges {
		out = append(out, to)
	}
	sortSlice(out)
	return out
}

func sortedCycleKeys(cycles map[string][]serviceMethod) []string {
	out := make([]string, 0, len(cycles))
	for key := range cycles {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

func sortSlice(nodes []serviceMethod) {
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].String() < nodes[j].String() })
}

type callEdge struct {
	pos token.Pos
}

// findHandlerInterfaces scans this package and its direct imports for named
// interfaces shaped like generated handler interfaces: name ends in "Atom" or
// "Element" (with a non-empty service prefix) and the interface embeds an
// interface named "Atomos".
func findHandlerInterfaces(pass *analysis.Pass) []handlerIface {
	var out []handlerIface
	scopes := []*types.Scope{pass.Pkg.Scope()}
	for _, imp := range pass.Pkg.Imports() {
		scopes = append(scopes, imp.Scope())
	}
	for _, scope := range scopes {
		for _, name := range scope.Names() {
			tn, ok := scope.Lookup(name).(*types.TypeName)
			if !ok || tn.IsAlias() {
				continue
			}
			named, ok := tn.Type().(*types.Named)
			if !ok {
				continue
			}
			iface, ok := named.Underlying().(*types.Interface)
			if !ok {
				continue
			}
			service, kind, ok := trimHandlerSuffix(name)
			if !ok || !embedsAtomos(iface, 0) {
				continue
			}
			hi := handlerIface{service: service, kind: kind, iface: iface.Complete()}
			hi.methods = explicitMethodNames(hi.iface)
			if len(hi.methods) > 0 {
				out = append(out, hi)
			}
		}
	}
	return out
}

// explicitMethodNames returns the method names declared directly on the
// interface (excluding methods promoted from embedded interfaces). Generated
// handler interfaces only declare their RPC methods explicitly; everything
// else (GetIDInfo, Cosmos, ...) comes from the embedded Atomos.
func explicitMethodNames(iface *types.Interface) []string {
	embedded := map[string]bool{}
	for i := 0; i < iface.NumEmbeddeds(); i++ {
		if eiface, ok := iface.EmbeddedType(i).Underlying().(*types.Interface); ok {
			for j := 0; j < eiface.NumMethods(); j++ {
				embedded[eiface.Method(j).Name()] = true
			}
		}
	}
	var out []string
	for i := 0; i < iface.NumMethods(); i++ {
		m := iface.Method(i)
		if m != nil && !embedded[m.Name()] {
			out = append(out, m.Name())
		}
	}
	return out
}

// trimHandlerSuffix splits a generated interface name into its service name
// and kind. ok=false when the name does not end in Atom/Element or has no
// service prefix.
func trimHandlerSuffix(name string) (service, kind string, ok bool) {
	for _, suffix := range []string{"Atom", "Element"} {
		if strings.HasSuffix(name, suffix) && len(name) > len(suffix) {
			return strings.TrimSuffix(name, suffix), suffix, true
		}
	}
	return "", "", false
}

// embedsAtomos reports whether the interface embeds (transitively) an
// interface named "Atomos" — the framework base interface every generated
// handler interface embeds.
func embedsAtomos(iface *types.Interface, depth int) bool {
	if depth > 8 {
		return false
	}
	for i := 0; i < iface.NumEmbeddeds(); i++ {
		emb := iface.EmbeddedType(i)
		if named, ok := emb.(*types.Named); ok {
			if named.Obj().Name() == "Atomos" {
				return true
			}
			if sub, ok := named.Underlying().(*types.Interface); ok && embedsAtomos(sub, depth+1) {
				return true
			}
		}
	}
	return false
}

// implementsHandler reports whether T or *T implements the handler interface.
func implementsHandler(t *types.Named, iface *types.Interface) bool {
	return types.Implements(t, iface) || types.Implements(types.NewPointer(t), iface)
}

// receiverTypeName extracts the base type name of a method receiver
// ("atom" for `func (t *atom) M(...)`). Empty for non-identifier receivers.
func receiverTypeName(fn *ast.FuncDecl) string {
	if len(fn.Recv.List) == 0 || len(fn.Recv.List[0].Names) == 0 {
		return ""
	}
	// 解包指针/泛型实例化等形式,只取基础标识符。
	expr := fn.Recv.List[0].Type
	for {
		switch e := expr.(type) {
		case *ast.StarExpr:
			expr = e.X
		case *ast.IndexExpr:
			expr = e.X
		case *ast.Ident:
			return e.Name
		default:
			return ""
		}
	}
}

// collectSyncCallEdges walks a handler body and records typed sync-call edges
// into edges. Calls lexically inside a `go` statement are skipped: they run on
// another goroutine and do not block the mailbox.
func collectSyncCallEdges(pass *analysis.Pass, h handlerDecl, edges map[serviceMethod]map[serviceMethod]callEdge) {
	add := func(to serviceMethod, pos token.Pos) {
		// Same handler on another atom is not a static cycle: atom names are
		// runtime values, so A.M→B.M only loops if the names chain back at
		// runtime — the wait-graph's job. Only multi-node handler cycles are
		// reported here.
		if to == h.node {
			return
		}
		if edges[h.node] == nil {
			edges[h.node] = map[serviceMethod]callEdge{}
		}
		if _, exists := edges[h.node][to]; !exists {
			edges[h.node][to] = callEdge{pos: pos}
		}
	}
	ast.Inspect(h.fn.Body, func(n ast.Node) bool {
		switch n.(type) {
		case *ast.GoStmt:
			return false // runs on another goroutine; cannot deadlock this mailbox
		}
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if to, ok := syncCallTarget(pass, call); ok {
			add(to, call.Pos())
		}
		return true
	})
}

// syncCallTarget resolves a call expression to the handler it (potentially)
// invokes synchronously. Recognized shapes:
//
//   - method call on an ID client type (named type ending in
//     "AtomID"/"ElementID"): each method is an edge to (service, method).
//     SyncMessagingByName uses its literal message-name argument as the method.
//     Async variants (AsyncXxx names, callback-taking signatures) are excluded.
//   - package function Spawn<Svc>Atom(...): edge to (Svc, "Spawn").
//   - CosmosSpawnAtom(caller, elem, name, arg) with a literal elem name:
//     edge to (elem, "Spawn").
//
// Everything else (runtime-computed names, calls on non-client types) yields
// false.
func syncCallTarget(pass *analysis.Pass, call *ast.CallExpr) (serviceMethod, bool) {
	switch fn := call.Fun.(type) {
	case *ast.Ident:
		// Same-package function call, e.g. SpawnGreeterAtom(...).
		obj, ok := pass.TypesInfo.Uses[fn].(*types.Func)
		if !ok {
			return serviceMethod{}, false
		}
		return factoryTarget(obj.Name(), call)
	case *ast.SelectorExpr:
		if sel, ok := pass.TypesInfo.Selections[fn]; ok {
			// Method call on a value: edge only when the receiver type is an
			// ID client type.
			recv := derefNamed(sel.Recv())
			if recv == nil {
				return serviceMethod{}, false
			}
			service, kind, isClient := trimIDClientSuffix(recv.Obj().Name())
			if !isClient {
				return serviceMethod{}, false
			}
			method := fn.Sel.Name
			if strings.HasPrefix(method, "Async") || methodTakesCallback(pass, recv, method) {
				return serviceMethod{}, false
			}
			if method == "SyncMessagingByName" {
				// SyncMessagingByName(caller, name, in, ...): the handler is
				// the literal name argument.
				if len(call.Args) < 2 {
					return serviceMethod{}, false
				}
				lit, ok := stringLiteral(call.Args[1])
				if !ok {
					return serviceMethod{}, false
				}
				method = lit
			}
			return serviceMethod{service: service, kind: kind, method: method}, true
		}
		// Package-qualified function call, e.g. api.SpawnGreeterAtom(...).
		if id, ok := fn.X.(*ast.Ident); ok {
			if _, isPkg := pass.TypesInfo.Uses[id].(*types.PkgName); isPkg {
				return factoryTarget(fn.Sel.Name, call)
			}
		}
	}
	return serviceMethod{}, false
}

// factoryTarget maps a generated factory function name (or framework spawn
// entry) to the handler it invokes synchronously.
func factoryTarget(name string, call *ast.CallExpr) (serviceMethod, bool) {
	if strings.HasPrefix(name, "Spawn") && strings.HasSuffix(name, "Atom") &&
		len(name) > len("Spawn")+len("Atom") {
		return serviceMethod{service: strings.TrimSuffix(strings.TrimPrefix(name, "Spawn"), "Atom"), kind: "Atom", method: "Spawn"}, true
	}
	if name == "CosmosSpawnAtom" {
		// CosmosSpawnAtom(caller, elem, name, arg): elem at index 1.
		if len(call.Args) >= 2 {
			if lit, ok := stringLiteral(call.Args[1]); ok {
				return serviceMethod{service: lit, kind: "Atom", method: "Spawn"}, true
			}
		}
	}
	return serviceMethod{}, false
}

// trimIDClientSuffix splits an ID client type name ("GreeterAtomID") into its
// service name ("Greeter") and kind. ok=false for other names.
func trimIDClientSuffix(name string) (service, kind string, ok bool) {
	for _, suffix := range []string{"AtomID", "ElementID"} {
		if strings.HasSuffix(name, suffix) && len(name) > len(suffix) {
			return strings.TrimSuffix(name, suffix), strings.TrimSuffix(suffix, "ID"), true
		}
	}
	return "", "", false
}

// methodTakesCallback reports whether the named method on recv has a
// function-typed parameter — the signature shape of generated async clients.
func methodTakesCallback(pass *analysis.Pass, recv *types.Named, method string) bool {
	obj, _, _ := types.LookupFieldOrMethod(recv, true, pass.Pkg, method)
	sig, ok := obj.(*types.Func)
	if !ok {
		return false
	}
	fnSig, ok := sig.Type().(*types.Signature)
	if !ok {
		return false
	}
	for i := 0; i < fnSig.Params().Len(); i++ {
		if _, ok := fnSig.Params().At(i).Type().Underlying().(*types.Signature); ok {
			return true
		}
	}
	return false
}

// stringLiteral extracts a basic string literal, if expr is one.
func stringLiteral(expr ast.Expr) (string, bool) {
	if lit, ok := expr.(*ast.BasicLit); ok && lit.Kind == token.STRING {
		return strings.Trim(lit.Value, "`\""), true
	}
	return "", false
}

func derefNamed(t types.Type) *types.Named {
	if ptr, ok := t.(*types.Pointer); ok {
		t = ptr.Elem()
	}
	named, ok := t.(*types.Named)
	if !ok {
		return nil
	}
	return named
}

// canonicalCycleKey produces the dedup key for a cycle: its canonical rotation
// joined with "→".
func canonicalCycleKey(cycle []serviceMethod) string {
	names := make([]string, len(cycle))
	for i, n := range rotateCycle(cycle) {
		names[i] = n.String()
	}
	return strings.Join(names, "→")
}

func joinNames(names []serviceMethod, sep string) string {
	strs := make([]string, len(names))
	for i, n := range names {
		strs[i] = n.String()
	}
	return strings.Join(strs, sep)
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

// analyzeSelfCall tracks IDs resolved with <recv>.GetIDInfo().Atom and reports
// only when one of those IDs is used for an actual synchronous handler call.
// Merely obtaining the ID, async calls, and calls inside go statements are safe.
func analyzeSelfCall(pass *analysis.Pass, h handlerDecl, knownHandlers map[serviceMethod]bool) {
	recvName := ""
	if h.fn.Recv != nil && len(h.fn.Recv.List) > 0 && len(h.fn.Recv.List[0].Names) > 0 {
		recvName = h.fn.Recv.List[0].Names[0].Name
	}
	selfIDs := map[*types.Var]string{}

	ast.Inspect(h.fn.Body, func(n ast.Node) bool {
		if _, ok := n.(*ast.GoStmt); ok {
			return false
		}
		if stmt, ok := n.(*ast.AssignStmt); ok && len(stmt.Rhs) == 1 && len(stmt.Lhs) > 0 {
			if call, ok := stmt.Rhs[0].(*ast.CallExpr); ok {
				factoryName := factoryFuncName(call)
				nameArgIdx := factoryNameArgIndex(factoryName, call)
				if nameArgIdx >= 0 && nameArgIdx < len(call.Args) && exprReferencesSelf(call.Args[nameArgIdx], recvName) {
					if v := assignedVar(pass, stmt.Lhs[0]); v != nil {
						selfIDs[v] = factoryName
					}
				}
			}
			return true
		}

		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		fn, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		id, ok := fn.X.(*ast.Ident)
		if !ok {
			return true
		}
		v, ok := pass.TypesInfo.Uses[id].(*types.Var)
		if !ok {
			return true
		}
		factoryName, selfID := selfIDs[v]
		if !selfID {
			return true
		}
		target, ok := syncCallTarget(pass, call)
		if !ok || !knownHandlers[target] || target.service != h.node.service || target.kind != h.node.kind {
			return true
		}
		pass.Reportf(call.Pos(),
			"sync handler %s sync-calls its own atom (%s from %s with self atom name) — this sync call always deadlocks; use async or restructure",
			h.node, target, factoryName)
		return true
	})
}

func assignedVar(pass *analysis.Pass, expr ast.Expr) *types.Var {
	id, ok := expr.(*ast.Ident)
	if !ok {
		return nil
	}
	if v, ok := pass.TypesInfo.Defs[id].(*types.Var); ok {
		return v
	}
	v, _ := pass.TypesInfo.Uses[id].(*types.Var)
	return v
}

// factoryFuncName returns the called function's name for factory-shape calls
// (identifier or package-qualified selector).
func factoryFuncName(call *ast.CallExpr) string {
	switch fn := call.Fun.(type) {
	case *ast.Ident:
		return fn.Name
	case *ast.SelectorExpr:
		return fn.Sel.Name
	}
	return ""
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
// self/receiver via the canonical self-resolution pattern
// `<recv>.GetIDInfo().Atom`.
func exprReferencesSelf(expr ast.Expr, recvName string) bool {
	// Match <X>.GetIDInfo().Atom — the canonical self-resolution pattern.
	// The AST for `self.GetIDInfo().Atom` is a SelectorExpr:
	//   Sel=Atom, X=(SelectorExpr Sel=GetIDInfo ... X=<ident or selector>)
	if sel, ok := expr.(*ast.SelectorExpr); ok {
		if sel.Sel.Name == "Atom" {
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
