package main

import (
	"github.com/otterize/intents-operator/linters"
	"golang.org/x/tools/go/analysis"
)

func New(_ any) ([]*analysis.Analyzer, error) {
	return []*analysis.Analyzer{linters.ErrorsNewAnalyzer}, nil
}
