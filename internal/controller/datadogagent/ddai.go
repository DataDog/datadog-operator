// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagent

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/global"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/override"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/comparison"
)

func (r *Reconciler) generateDDAIFromDDA(dda *v2alpha1.DatadogAgent) (*v1alpha1.DatadogAgentInternal, error) {
	ddai := &v1alpha1.DatadogAgentInternal{}
	// Object meta
	if err := generateObjMetaFromDDA(dda, ddai, r.scheme); err != nil {
		return nil, err
	}
	// Spec
	if err := generateSpecFromDDA(dda, ddai); err != nil {
		return nil, err
	}

	// Set hash
	if _, err := comparison.SetMD5GenerationAnnotation(&ddai.ObjectMeta, ddai.Spec, constants.MD5DDAIDeploymentAnnotationKey); err != nil {
		return nil, err
	}

	return ddai, nil
}

func generateObjMetaFromDDA(dda *v2alpha1.DatadogAgent, ddai *v1alpha1.DatadogAgentInternal, scheme *runtime.Scheme) error {
	ddai.ObjectMeta = metav1.ObjectMeta{
		Name:        dda.Name,
		Namespace:   dda.Namespace,
		Labels:      getDDAILabels(dda),
		Annotations: dda.Annotations,
	}
	if err := object.SetOwnerReference(dda, ddai, scheme); err != nil {
		return err
	}
	return nil
}

func generateSpecFromDDA(dda *v2alpha1.DatadogAgent, ddai *v1alpha1.DatadogAgentInternal) error {
	ddai.Spec = *dda.Spec.DeepCopy()
	global.SetGlobalFromDDA(dda, ddai.Spec.Global)
	override.SetOverrideFromDDA(dda, &ddai.Spec)
	return nil
}

// getDDAILabels adds the following labels to the DDAI:
// - all DDA labels
// - agent.datadoghq.com/datadogagent: <dda-name>
func getDDAILabels(dda metav1.Object) map[string]string {
	labels := make(map[string]string)
	for k, v := range dda.GetLabels() {
		labels[k] = v
	}
	labels[apicommon.DatadogAgentNameLabelKey] = dda.GetName()
	return labels
}

func (r *Reconciler) cleanUpUnusedDDAIs(ctx context.Context, validDDAIs []*v1alpha1.DatadogAgentInternal) error {
	validDDAIMap := make(map[string]struct{}, len(validDDAIs))
	for _, ddai := range validDDAIs {
		validDDAIMap[fmt.Sprintf("%s/%s", ddai.Namespace, ddai.Name)] = struct{}{}
	}
	ddaiList := &v1alpha1.DatadogAgentInternalList{}
	if err := r.client.List(ctx, ddaiList); err != nil {
		return err
	}
	for _, ddai := range ddaiList.Items {
		if _, isValid := validDDAIMap[fmt.Sprintf("%s/%s", ddai.Namespace, ddai.Name)]; !isValid {
			r.log.Info("Deleting unused DDAI", "namespace", ddai.Namespace, "name", ddai.Name)
			if err := r.client.Delete(ctx, &ddai); err != nil {
				return err
			}
		}
	}

	return nil
}
