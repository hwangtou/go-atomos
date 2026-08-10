// Package deferrelease is a go vet analyzer that flags ID-returning factory
// calls whose result is assigned to a variable but not released via
// `defer <id>.Release()`, WithID, or WithRebind in the same function — a likely
// IDTracker leak.
//
// It recognizes the go-atomos generated/runtime factories that return a real
// *IDTracker (Atom IDs), and excludes Element IDs (nil tracker). Only non-test
// files are checked: tests deliberately use bare non-deferred Release for
// scoping.
package deferrelease

import (
	"go/ast"
	"strings"

	"golang.org/x/tools/go/analysis"
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

func run(pass *analysis.Pass) (interface{}, error) {
	for _, file := range pass.Files {
		if isTestFile(pass, file) {
			continue
		}
		if isGenerated(file) {
			continue
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			analyzeFunc(pass, fn)
		}
	}
	return nil, nil
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
	// Check by file name (not package path): test files in the same package as
	// production code (single-package testing, common in Go) still need skipping.
	pos := pass.Fset.Position(file.Pos())
	return strings.HasSuffix(pos.Filename, "_test.go")
}

func analyzeFunc(pass *analysis.Pass, fn *ast.FuncDecl) {
	released := map[string]bool{}
	exempted := map[string]bool{}
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		switch stmt := n.(type) {
		case *ast.DeferStmt:
			if name := receiverOfRelease(stmt.Call); name != "" {
				released[name] = true
			}
		case *ast.ExprStmt:
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
			targets := targetsFor(stmt, i, info)
			for _, tgt := range targets {
				if tgt == "_" {
					continue
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

func noteExempt(call *ast.CallExpr, exempted map[string]bool) {
	fnName := callFuncName(call)
	switch fnName {
	case "WithID":
		if len(call.Args) >= 1 {
			if name := baseIdentName(call.Args[0]); name != "" {
				exempted[name] = true
			}
		}
	case "WithRebind":
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

type factoryInfo struct {
	factoryName     string
	releasableIndex int
}

func factoryReturnInfo(call *ast.CallExpr) *factoryInfo {
	name := callFuncName(call)
	if isAtomIDFactoryName(name) {
		return &factoryInfo{factoryName: name, releasableIndex: 0}
	}
	if name == "CosmosGetAtomID" || name == "CosmosSpawnAtom" {
		return &factoryInfo{factoryName: name, releasableIndex: 1}
	}
	return nil
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

func targetsFor(stmt *ast.AssignStmt, rhsIdx int, info *factoryInfo) []string {
	if len(stmt.Rhs) == 1 && len(stmt.Lhs) > 1 {
		if info.releasableIndex < len(stmt.Lhs) {
			return []string{identName(stmt.Lhs[info.releasableIndex])}
		}
		return nil
	}
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
