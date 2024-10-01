package main

import (
	"os"

	"github.com/spf13/cobra"
)

const (
	FormatCUE   = "cue"
	FormatThema = "thema"
)

var rootCmd = &cobra.Command{
	Use:   "grafana-app-sdk <command>",
	Short: "A tool for working with grafana apps, used for generating code from CUE kinds, creating project boilerplate, and running local deployments",
	Long:  "A tool for working with grafana apps, used for generating code from CUE kinds, creating project boilerplate, and running local deployments",
}

const themaWarning = `
!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!
!!! WARNING: --format=thema is deprecated and will be removed in a future release.          !!!
!!! Please use the (default) CUE format instead. For more details, see                      !!!
!!! https://github.com/grafana/grafana-app-sdk/blob/main/docs/custom-kinds/writing-kinds.md !!!
!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!`

func main() {
	rootCmd.PersistentFlags().StringP("cuepath", "c", "kinds", "Path to directory with cue.mod")
	rootCmd.PersistentFlags().StringSliceP("selectors", "s", []string{}, "selectors")
	rootCmd.PersistentFlags().StringP("format", "f", "cue", "Format in which kinds are written for this project (currently allowed values are 'cue')")

	setupVersionCmd()
	setupGenerateCmd()
	setupValidateCmd()
	setupProjectCmd()

	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(generateCmd)
	rootCmd.AddCommand(validateCmd)
	rootCmd.AddCommand(projectCmd)

	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}
