// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagent

import (
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	datadoghqv2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/appsec"
	"github.com/DataDog/datadog-operator/pkg/condition"
)

// deprecatedConfigCheck reports whether one deprecated configuration surface is in use
// on a DatadogAgent, and names the CRD path that replaces it.
type deprecatedConfigCheck struct {
	// name identifies the deprecated surface, as a user would recognize it.
	name string
	// replacement is the configuration path that supersedes it.
	replacement string
	// inUse reports whether the surface is still in use on this DatadogAgent.
	inUse func(dda *datadoghqv2alpha1.DatadogAgent) bool
}

// deprecatedConfigChecks lists every deprecated configuration surface surfaced through
// the DeprecatedConfigInUse condition. Add an entry here when deprecating a surface so
// users can discover it from `kubectl describe datadogagent` rather than operator logs.
//
// Iteration order is the declaration order below, which keeps the condition message
// stable across reconciles.
var deprecatedConfigChecks = []deprecatedConfigCheck{
	{
		name:        "agent.datadoghq.com/appsec.* annotations",
		replacement: appsec.DeprecatedConfigReplacement,
		inUse: func(dda *datadoghqv2alpha1.DatadogAgent) bool {
			return appsec.HasDeprecatedAnnotations(dda.GetAnnotations())
		},
	},
}

// setDeprecatedConfigStatus updates the DeprecatedConfigInUse condition to reflect which
// deprecated configuration surfaces the DatadogAgent still relies on.
//
// The condition only exists while at least one deprecated surface is in use, and is
// removed once none are: its presence is the signal, so there is no value in leaving a
// False copy behind on every DatadogAgent that finished migrating or never used one.
//
// It is informational, not an error: a deprecated surface keeps working until the release
// that removes it. It exists because the operator log is not a reliable channel for
// reaching the owner of a DatadogAgent, and this CRD has no validating webhook that could
// report the same thing at apply time.
func setDeprecatedConfigStatus(dda *datadoghqv2alpha1.DatadogAgent, status *datadoghqv2alpha1.DatadogAgentStatus, now metav1.Time) {
	if dda == nil || status == nil {
		return
	}

	var messages []string
	for _, check := range deprecatedConfigChecks {
		if check.inUse(dda) {
			messages = append(messages, fmt.Sprintf("%s (use %s instead)", check.name, check.replacement))
		}
	}

	if len(messages) == 0 {
		condition.DeleteDatadogAgentStatusCondition(status, common.DeprecatedConfigInUseConditionType)
		return
	}

	condition.UpdateDatadogAgentStatusConditions(
		status, now, common.DeprecatedConfigInUseConditionType, metav1.ConditionTrue,
		"DeprecatedConfigInUse",
		"Deprecated configuration in use: "+strings.Join(messages, "; "), false,
	)
}
