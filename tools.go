//go:build tools
// +build tools

// This file pins the versions of development tools that symskills invokes via
// `go run <module>@<version>`. Listing them here makes the versions explicit in
// go.mod and lets Dependabot's gomod ecosystem keep them current.
//
// The corresponding CI/Makefile entries must reference the same versions.
package tools

import (
	_ "golang.org/x/vuln/cmd/govulncheck"
	_ "honnef.co/go/tools/cmd/staticcheck"
)
