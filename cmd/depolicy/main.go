package main

import (
	"os"

	"golang.org/x/tools/go/analysis/singlechecker"

	"github.com/satorunooshie/depolicy"
)

func main() {
	if depolicy.IsVetToolInvocation(os.Args[1:]) {
		singlechecker.Main(depolicy.Analyzer)
		return
	}
	os.Exit(depolicy.CLI{}.Run(os.Args[1:]))
}
