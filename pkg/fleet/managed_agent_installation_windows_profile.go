// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fleet

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	v2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

const managedAgentInstallationWindowsProfileName = "datadog-agent-windows"

var managedAgentInstallationWindowsProfileKey = types.NamespacedName{
	Namespace: fleetDatadogAgentNamespace,
	Name:      managedAgentInstallationWindowsProfileName,
}

func (d *Daemon) ensureManagedAgentInstallationWindowsProfile(ctx context.Context, dda *v2alpha1.DatadogAgent) error {
	wanted := d.managedAgentInstallationWindowsProfile(dda)
	current := &v1alpha1.DatadogAgentProfile{}
	getErr := d.managedAgentInstallationReader().Get(ctx, managedAgentInstallationWindowsProfileKey, current)
	if apierrors.IsNotFound(getErr) {
		if createErr := d.client.Create(ctx, wanted, client.FieldOwner("fleet-daemon")); createErr != nil {
			return fmt.Errorf("create Windows DatadogAgentProfile: %w", createErr)
		}
		return nil
	}
	if getErr != nil {
		return fmt.Errorf("read Windows DatadogAgentProfile: %w", getErr)
	}
	return d.validateManagedAgentInstallationWindowsProfile(current, dda)
}

func (d *Daemon) managedAgentInstallationWindowsProfile(dda *v2alpha1.DatadogAgent) *v1alpha1.DatadogAgentProfile {
	return &v1alpha1.DatadogAgentProfile{
		TypeMeta: metav1.TypeMeta{
			APIVersion: v1alpha1.GroupVersion.String(),
			Kind:       "DatadogAgentProfile",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: managedAgentInstallationWindowsProfileKey.Namespace,
			Name:      managedAgentInstallationWindowsProfileKey.Name,
			Labels: map[string]string{
				fleetManagedByLabel:                        fleetManagedByValue,
				fleetManagedAgentInstallationProviderLabel: string(d.managedAgentInstallationIdentity.Provider()),
				fleetInstallationIDLabel:                   d.managedAgentInstallationIdentity.InstallationID(),
				fleetTargetIDLabel:                         d.managedAgentInstallationIdentity.TargetID(),
			},
			Annotations: map[string]string{
				kubernetes.ProviderAnnotationKey: kubernetes.WindowsProvider,
			},
			OwnerReferences: []metav1.OwnerReference{controllerOwnerReference(
				v2alpha1.GroupVersion.String(), "DatadogAgent", dda.Name, dda.UID,
			)},
		},
		Spec: v1alpha1.DatadogAgentProfileSpec{
			ProfileAffinity: &v1alpha1.ProfileAffinity{
				ProfileNodeAffinity: []corev1.NodeSelectorRequirement{{
					Key:      corev1.LabelOSStable,
					Operator: corev1.NodeSelectorOpIn,
					Values:   []string{string(corev1.Windows)},
				}},
			},
			Config: &v2alpha1.DatadogAgentSpec{},
		},
	}
}

func (d *Daemon) validateManagedAgentInstallationWindowsProfile(profile *v1alpha1.DatadogAgentProfile, dda *v2alpha1.DatadogAgent) error {
	wanted := d.managedAgentInstallationWindowsProfile(dda)
	if profile.DeletionTimestamp != nil {
		return &stateDoesntMatchError{msg: fmt.Sprintf("Windows DatadogAgentProfile %s/%s is terminating", profile.Namespace, profile.Name)}
	}
	if profile.Labels[fleetManagedByLabel] != fleetManagedByValue ||
		profile.Labels[fleetManagedAgentInstallationProviderLabel] != string(d.managedAgentInstallationIdentity.Provider()) ||
		profile.Labels[fleetInstallationIDLabel] != d.managedAgentInstallationIdentity.InstallationID() ||
		profile.Labels[fleetTargetIDLabel] != d.managedAgentInstallationIdentity.TargetID() {
		return &stateDoesntMatchError{msg: fmt.Sprintf("Windows DatadogAgentProfile %s/%s has invalid managed Agent installation ownership", profile.Namespace, profile.Name)}
	}
	if err := requireManagedAgentInstallationWindowsProfileOwner(profile.OwnerReferences, wanted.OwnerReferences[0], dda.UID != ""); err != nil {
		return &stateDoesntMatchError{msg: fmt.Sprintf("Windows DatadogAgentProfile %s/%s is not owned by DatadogAgent %s/%s", profile.Namespace, profile.Name, dda.Namespace, dda.Name)}
	}
	if profile.Annotations[kubernetes.ProviderAnnotationKey] != kubernetes.WindowsProvider || !apiequality.Semantic.DeepEqual(profile.Spec, wanted.Spec) {
		return &stateDoesntMatchError{msg: fmt.Sprintf("Windows DatadogAgentProfile %s/%s differs from the pinned managed Agent installation configuration", profile.Namespace, profile.Name)}
	}
	return nil
}

func requireManagedAgentInstallationWindowsProfileOwner(owners []metav1.OwnerReference, want metav1.OwnerReference, requireUID bool) error {
	for _, owner := range owners {
		if owner.APIVersion != want.APIVersion || owner.Kind != want.Kind || owner.Name != want.Name || owner.Controller == nil || !*owner.Controller {
			continue
		}
		if requireUID && owner.UID != want.UID {
			continue
		}
		if owner.UID != "" {
			return nil
		}
	}
	return fmt.Errorf("required controller owner reference is missing")
}

func (d *Daemon) validateManagedAgentInstallationWindowsProfileExists(ctx context.Context, dda *v2alpha1.DatadogAgent) error {
	profile := &v1alpha1.DatadogAgentProfile{}
	if err := d.managedAgentInstallationReader().Get(ctx, managedAgentInstallationWindowsProfileKey, profile); err != nil {
		return fmt.Errorf("read Windows DatadogAgentProfile: %w", err)
	}
	return d.validateManagedAgentInstallationWindowsProfile(profile, dda)
}
