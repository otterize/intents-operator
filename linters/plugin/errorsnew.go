package main

import (
	"github.com/otterize/intents-operator/linters/errorsnew"
	"golang.org/x/tools/go/analysis"
)

func New(_ any) ([]*analysis.Analyzer, error) {
	return []*analysis.Analyzer{errorsnew.Analyzer}, nil
}
