//go:build tools
// +build tools

package tools

import (
	// Code generators built at runtime.
	_ "k8s.io/kube-openapi/cmd/openapi-gen"
)
