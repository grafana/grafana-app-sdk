package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"log"

	"cuelang.org/go/cue/cuecontext"
	"github.com/grafana/thema"

	"github.com/grafana/grafana-app-sdk/codegen"
)

//go:embed example.cue cue.mod/module.cue
var modFS embed.FS

// Using go generate, this will turn all the supported selectors in all cue files in our current directory into:
// * go files defining the type struct, and a resource.Object-implementing struct encapsulating it
// * YAML Custom Resource Definition files for each object
// Make sure you run a `make build` in the project root so that `target` is populated
//
//go:generate ../../../target/grafana-app-sdk generate all --crdencoding=yaml --cuepath .
func main() {
	// Create a new CustomKindParser using the default thema_cue library and our modFS that has the cue files embedded
	g, err := codegen.NewCustomKindParser(thema.NewRuntime(cuecontext.New()), modFS)
	if err != nil {
		log.Panicln(err)
	}

	// Just generate the JSON CustomResourceDefinition file for the myObject selector:
	files, err := g.Generate(codegen.CRDGenerator(json.Marshal, "json"), "myObject")
	// Print the contents to the console, rather than writing out to disk
	// You could also use this to write to a kubernetes API
	for _, f := range files {
		fmt.Printf("%s: %s\n", f.RelativePath, string(f.Data))
	}
}
