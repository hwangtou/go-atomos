package main

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestAnalyzer(t *testing.T) {
	// analysistest loads packages under testdata/src as if they were real
	// packages. The leak/ok fixtures are self-contained (no external imports),
	// so no module/vendor setup is needed.
	analysistest.Run(t, analysistest.TestData(), Analyzer, "leak", "ok")
}
