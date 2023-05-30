package main

import (
	"fmt"
	"os"

	"cuelang.org/go/cue/cuecontext"
	"github.com/grafana/thema"
	"github.com/spf13/cobra"

	"github.com/grafana/grafana-app-sdk/codegen"
)

var validateCmd = &cobra.Command{
	Use:  "validate",
	RunE: validate,
}

func setupValidateCmd() {
	// Do nothing currently
}

//nolint:revive
func validate(cmd *cobra.Command, args []string) error {
	cuePath, err := cmd.Flags().GetString("cuepath")
	if err != nil {
		return err
	}

	selectors, err := cmd.Flags().GetStringSlice("selectors")
	if err != nil {
		return err
	}

	generator, err := codegen.NewCustomKindParser(thema.NewRuntime(cuecontext.New()), os.DirFS(cuePath))
	if err != nil {
		return err
	}

	vErrs, err := generator.Validate(selectors...)
	if err != nil {
		return err
	}
	if len(vErrs) != 0 {
		for sel, errs := range vErrs {
			fmt.Printf("%s:\n", sel)
			for _, e := range errs.Errors {
				fmt.Printf(" * %s\n", e.Error())
			}
		}
		os.Exit(1)
	}
	return nil
}
