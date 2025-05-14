// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagent

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	datadoghqv2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/global"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/override"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/comparison"
)

const (
	ddaiNameTemplate = "%s-ddai"
)

func (r *Reconciler) generateDDAIFromDDA(dda *datadoghqv2alpha1.DatadogAgent) (*datadoghqv1alpha1.DatadogAgentInternal, error) {
	ddai := &datadoghqv1alpha1.DatadogAgentInternal{}
	// Object meta
	if err := r.generateObjMetaFromDDA(dda, ddai); err != nil {
		return nil, err
	}
	// Spec
	generateSpecFromDDA(dda, ddai)

	// Set hash
	if _, err := comparison.SetMD5GenerationAnnotation(&ddai.ObjectMeta, ddai.Spec, constants.MD5DDAIDeploymentAnnotationKey); err != nil {
		return nil, err
	}

	return ddai, nil
}

func (r *Reconciler) generateObjMetaFromDDA(dda *datadoghqv2alpha1.DatadogAgent, ddai *datadoghqv1alpha1.DatadogAgentInternal) error {
	ddai.ObjectMeta = metav1.ObjectMeta{
		Name:        getDDAINameFromDDA(dda.Name),
		Namespace:   dda.Namespace,
		Annotations: dda.Annotations,
	}
	ddai.ObjectMeta.Labels = getDDAILabelsFromDDA(ddai, dda.Name)
	if err := object.SetOwnerReference(dda, ddai, r.scheme); err != nil {
		return err
	}
	return nil
}

func getDDAINameFromDDA(ddaName string) string {
	return fmt.Sprintf(ddaiNameTemplate, ddaName)
}

func getDDAILabelsFromDDA(ddai metav1.Object, ddaName string) map[string]string {
	// TODO: set `version` label value, currently ""
	labels := common.GetDefaultLabels(ddai, "datadogagentinternal", ddai.GetName(), "")
	labels[apicommon.OperatorDatadogAgentLabelKey] = ddaName
	return labels
}

func generateSpecFromDDA(dda *datadoghqv2alpha1.DatadogAgent, ddai *datadoghqv1alpha1.DatadogAgentInternal) {
	ddai.Spec = dda.Spec
	global.SetGlobalFromDDA(dda, ddai.Spec.Global)
	override.SetOverrideFromDDA(dda, ddai.Spec.Override)
}
