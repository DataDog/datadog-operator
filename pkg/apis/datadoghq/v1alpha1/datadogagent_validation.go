// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package v1alpha1

import (
	"fmt"

	utilserrors "k8s.io/apimachinery/pkg/util/errors"
)

// IsValidDatadogAgent use to check if a DatadogAgentSpec is valid
func IsValidDatadogAgent(spec *DatadogAgentSpec) error {
	var errs []error
	var err error
	if spec.Agent != nil {
		if spec.Agent.CustomConfig != nil {
			if err = IsValidCustomConfigSpec(spec.Agent.CustomConfig); err != nil {
				errs = append(errs, fmt.Errorf("invalid spec.agent.customConfig, err: %v", err))
			}
		}
	}

	if spec.ClusterAgent != nil {
		if spec.ClusterAgent.CustomConfig != nil {
			if err = IsValidCustomConfigSpec(spec.ClusterAgent.CustomConfig); err != nil {
				errs = append(errs, fmt.Errorf("invalid spec.clusterAgent.customConfig, err: %v", err))
			}
		}
	}

	if spec.ClusterChecksRunner != nil {
		if spec.ClusterChecksRunner.CustomConfig != nil {
			if err = IsValidCustomConfigSpec(spec.ClusterChecksRunner.CustomConfig); err != nil {
				errs = append(errs, fmt.Errorf("invalid spec.clusterChecksRunner.customConfig, err: %v", err))
			}
		}
	}

	return utilserrors.NewAggregate(errs)
}

// IsValidCustomConfigSpec used to check if a CustomConfigSpec is properly set
func IsValidCustomConfigSpec(ccs *CustomConfigSpec) error {
	if ccs.ConfigData != nil && ccs.ConfigMap != nil {
		return fmt.Errorf("'configData' and 'configMap' should not be set at the same time")
	}
	return nil
}
