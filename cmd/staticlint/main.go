package main

import (
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/multichecker"
	"golang.org/x/tools/go/analysis/passes/assign"
	"golang.org/x/tools/go/analysis/passes/copylock"
	"golang.org/x/tools/go/analysis/passes/loopclosure"
	"golang.org/x/tools/go/analysis/passes/lostcancel"
	"golang.org/x/tools/go/analysis/passes/nilness"
	"golang.org/x/tools/go/analysis/passes/printf"
	"golang.org/x/tools/go/analysis/passes/shadow"
	"honnef.co/go/tools/staticcheck"
	"strings"
)

func main() {
	var analyzers = []*analysis.Analyzer{
		assign.Analyzer,
		shadow.Analyzer,
		printf.Analyzer,
		nilness.Analyzer,
		lostcancel.Analyzer,
		copylock.Analyzer,
		loopclosure.Analyzer,
	}

	for _, a := range staticcheck.Analyzers {
		if strings.HasPrefix(a.Analyzer.Name, "SA") {
			analyzers = append(analyzers, a.Analyzer)
		}
	}

	multichecker.Main(analyzers...)
}
