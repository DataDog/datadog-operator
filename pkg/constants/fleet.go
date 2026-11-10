// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package constants

// Fleet Automation managed-install labels. They live here — not in pkg/fleet —
// because both pkg/fleet and internal/controller/datadogagent need them, and
// pkg/fleet's integration tests import the controllers, so the controllers must
// not import pkg/fleet back.
const (
	// FleetManagedByLabel marks a DatadogAgent as a Fleet Automation
	// managed-install.
	FleetManagedByLabel = "fleet.datadoghq.com/managed-by"
	// FleetManagedByValue is the managed-by label value Fleet Automation stamps.
	FleetManagedByValue = "fleet-automation"
)
