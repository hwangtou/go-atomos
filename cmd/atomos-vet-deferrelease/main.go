// Command atomos-vet-deferrelease is a go vet analyzer that flags ID-returning
// factory calls whose result is assigned to a variable but not released via
// `defer <id>.Release()`, WithID, or WithRebind in the same function — a likely
// IDTracker leak.
//
// It recognizes the go-atomos generated/runtime factories that return a real
// *IDTracker (Atom IDs), and excludes Element IDs (nil tracker). Only non-test
// files are checked: tests deliberately use bare non-deferred Release for
// scoping.
//
// Run via:
//
//	go vet -vettool=$(which atomos-vet-deferrelease) ./...
package main

import (
	"go/ast"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/singlechecker"
)

const doc = `check that ID-returning go-atomos factories are followed by defer Release/WithID/WithRebind

The go-atomos framework returns an *IDTracker from Atom ID factories
(GetXxxAtomID, SpawnXxxAtom, CosmosGetAtomID, CosmosSpawnAtom) that MUST be
released (defer id.Release()) to avoid pinning an Atom in memory. This analyzer
flags assignments of those results that lack a same-function release path
(defer .Release(), WithID, or WithRebind). Element IDs (GetXxxElementID) carry
a nil tracker and are not flagged. Test files are skipped.`

var Analyzer = &analysis.Analyzer{
	Name: "deferrelease",
	Doc:  doc,
	Run:  run,
}

func main() { singlechecker.Main(Analyzer) }

func run(pass *analysis.Pass) (interface{}, error) {
	// Skip test files.
	if isTestFile(pass) {
		return nil, nil
	}
	for _, file := range pass.Files {
		// Skip generated files (e.g. *_atomos.pb.go). Generated factories
		// intentionally forward the tracker to the caller (ownership transfer),
		// which is a valid pattern we should not flag. The standard Go
		// "Code generated ... DO NOT EDIT." header marks these.
		if isGenerated(file) {
			continue
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			analyzeFunc(pass, file, fn)
		}
	}
	return nil, nil
}

// isGenerated reports whether the file carries the standard
// "Code generated ... DO NOT EDIT." header, marking it as code-generated
// (and thus exempt from the analyzer's release-discipline check).
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

// isTestFile reports whether the pass is analyzing a _test.go package.
func isTestFile(pass *analysis.Pass) bool {
	// pass.Pkg.Path() for test files has a "_test" suffix in the package path.
	return strings.HasSuffix(pass.Pkg.Path(), "_test")
}

// analyzeFunc inspects one function body for tracker-returning assignments
// lacking a release path.
func analyzeFunc(pass *analysis.Pass, file *ast.File, fn *ast.FuncDecl) {
	// Collect identifiers that are released via defer .Release(), or passed to
	// WithID / WithRebind.
	released := map[string]bool{}
	exempted := map[string]bool{}
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		switch stmt := n.(type) {
		case *ast.DeferStmt:
			// defer <id>.Release()
			if name := receiverOfRelease(stmt.Call); name != "" {
				released[name] = true
			}
		case *ast.ExprStmt:
			// WithID(<tr>, ...) or WithRebind(...) bare call.
			if call, ok := stmt.X.(*ast.CallExpr); ok {
				noteExempt(call, exempted)
			}
		case *ast.AssignStmt:
			for _, expr := range stmt.Rhs {
				if call, ok := expr.(*ast.CallExpr); ok {
					noteExempt(call, exempted)
				}
			}
		case *ast.ReturnStmt:
			for _, expr := range stmt.Results {
				if call, ok := expr.(*ast.CallExpr); ok {
					noteExempt(call, exempted)
				}
			}
		}
		return true
	})

	// Now scan for assignments from tracker-returning factories and report any
	// whose target is not released/exempted.
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		stmt, ok := n.(*ast.AssignStmt)
		if !ok {
			return true
		}
		for i, rhs := range stmt.Rhs {
			call, ok := rhs.(*ast.CallExpr)
			if !ok {
				continue
			}
			info := factoryReturnInfo(call)
			if info == nil {
				continue
			}
			// For single-value RHS (generated GetXxxAtomID/SpawnXxxAtom): LHS is the ID.
			// For multi-value RHS (CosmosGetAtomID/CosmosSpawnAtom): LHS[i] corresponds
			// per-result; we care about the ID result (index 0) and the tracker result.
			targets := targetsFor(stmt, i, info)
			for _, tgt := range targets {
				if tgt == "_" {
					continue // explicitly discarded
				}
				if released[tgt] || exempted[tgt] {
					continue
				}
				pass.Reportf(call.Pos(),
					"%s from %s requires defer .Release(), WithID, or WithRebind in this function (possible IDTracker leak)",
					tgt, info.factoryName)
			}
		}
		return true
	})
}

// receiverOfRelease returns the identifier name in `x.Release()` call, or "".
func receiverOfRelease(call *ast.CallExpr) string {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Release" {
		return ""
	}
	ident, ok := sel.X.(*ast.Ident)
	if !ok {
		return ""
	}
	return ident.Name
}

// noteExempt records identifiers consumed by WithID/WithRebind helpers.
func noteExempt(call *ast.CallExpr, exempted map[string]bool) {
	fnName := callFuncName(call)
	switch fnName {
	case "WithID":
		// WithID(<tr>, ...): first arg is the tracker (or an expression whose
		// base identifier we can extract). Exempt that identifier.
		if len(call.Args) >= 1 {
			if name := baseIdentName(call.Args[0]); name != "" {
				exempted[name] = true
			}
		}
	case "WithRebind":
		// WithRebind(resolve, call): IDs obtained inside resolve and used only
		// in call are released internally. We exempt identifiers passed into
		// either closure conservatively by scanning the closure bodies for used
		// identifiers — but at minimum, any identifier appearing as a result of
		// a factory inside these closures is exempt. Simple heuristic: exempt
		// identifiers referenced inside the closure args.
		for _, arg := range call.Args {
			ast.Inspect(arg, func(n ast.Node) bool {
				if ident, ok := n.(*ast.Ident); ok && ident.Name != "" {
					exempted[ident.Name] = true
				}
				return true
			})
		}
	}
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

func baseIdentName(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.SelectorExpr:
		return e.Sel.Name
	case *ast.CallExpr:
		return baseIdentName(e.Fun)
	}
	return ""
}

// factoryInfo describes a tracker-returning factory call.
type factoryInfo struct {
	factoryName string
	// releasableIndex is the result index (0-based) holding the variable that
	// must be Released. For generated single-return factories (Get*AtomID /
	// Spawn*Atom) it is 0 (the ID embeds *IDTracker, so id.Release() works).
	// For runtime multi-return factories (CosmosGetAtomID / CosmosSpawnAtom) it
	// is 1 (the standalone *IDTracker result).
	releasableIndex int
}

func factoryReturnInfo(call *ast.CallExpr) *factoryInfo {
	name := callFuncName(call)
	// Generated free functions returning *XxxAtomID (single return value + error).
	if isAtomIDFactoryName(name) {
		return &factoryInfo{factoryName: name, releasableIndex: 0}
	}
	// Runtime methods returning (ID, *IDTracker, *Error): the tracker (index 1)
	// is the releasable surface.
	if name == "CosmosGetAtomID" || name == "CosmosSpawnAtom" {
		return &factoryInfo{factoryName: name, releasableIndex: 1}
	}
	return nil
}

// isAtomIDFactoryName reports whether name is a generated Atom ID factory
// (Get<Svc>AtomID or Spawn<Svc>Atom). Element factories (Get<Svc>ElementID)
// return false because their tracker is nil.
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

// targetsFor returns the identifier names that must be released, given an
// assignment statement, the rhs index, and the factory info.
func targetsFor(stmt *ast.AssignStmt, rhsIdx int, info *factoryInfo) []string {
	// Multi-value assignment from a single multi-return call:
	//   id, err            := GetFooAtomID(...)      // releasable=0 (id)
	//   id, tr, err        := CosmosGetAtomID(...)   // releasable=1 (tracker)
	if len(stmt.Rhs) == 1 && len(stmt.Lhs) > 1 {
		if info.releasableIndex < len(stmt.Lhs) {
			return []string{identName(stmt.Lhs[info.releasableIndex])}
		}
		return nil
	}
	// Single-value assignment: lhs[rhsIdx] = rhs[rhsIdx] (releasableIndex 0).
	if rhsIdx < len(stmt.Lhs) {
		return []string{identName(stmt.Lhs[rhsIdx])}
	}
	return nil
}

func identName(expr ast.Expr) string {
	ident, ok := expr.(*ast.Ident)
	if !ok {
		return ""
	}
	return ident.Name
}
