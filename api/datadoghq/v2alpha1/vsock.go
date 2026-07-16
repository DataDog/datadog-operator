// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package v2alpha1

// GetVSockConfig resolves the effective VSock configuration from the GlobalConfig,
// taking into account the deprecated UseVSock field.
//
// The new VSock section takes precedence: when the VSock section is set, the
// deprecated UseVSock field is ignored. When the VSock section is absent, UseVSock
// is honored for backward compatibility and maps to the "full" mode.
//
// It returns whether VSock communication is enabled and the mode that controls which
// Agent components communicate over VSock (defaulting to VSockModeFull).
func (g *GlobalConfig) GetVSockConfig() (enabled bool, mode VSockMode) {
	mode = VSockModeFull
	if g == nil {
		return false, mode
	}

	if g.VSock != nil {
		if g.VSock.Enabled != nil {
			enabled = *g.VSock.Enabled
		}
		if g.VSock.Mode != nil {
			mode = *g.VSock.Mode
		}
		return enabled, mode
	}

	// Backward compatibility with the deprecated UseVSock field.
	if g.UseVSock != nil {
		enabled = *g.UseVSock
	}
	return enabled, mode
}
