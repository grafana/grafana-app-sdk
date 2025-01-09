//go:build tools
// +build tools

package codegen

import (
	_ "k8s.io/code-generator/cmd/client-gen/generators"
)

// This file only exists to ensure we have access to the imported packages from the command-line.
