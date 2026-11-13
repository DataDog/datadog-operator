// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fleet

import (
	"context"
	"fmt"

	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	v2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
)

func (d *Daemon) ensureManagedAgentInstallationAgentProfiles(ctx context.Context, dda *v2alpha1.DatadogAgent, profiles []decodedManagedAgentInstallationAgentProfile) error {
	for _, profile := range profiles {
		wanted := d.managedAgentInstallationAgentProfile(dda, profile)
		profileKey := d.managedAgentInstallationKey(profile.Name)
		current := &v1alpha1.DatadogAgentProfile{}
		getErr := d.managedAgentInstallationReader().Get(ctx, profileKey, current)
		if apierrors.IsNotFound(getErr) {
			if createErr := d.client.Create(ctx, wanted, client.FieldOwner("fleet-daemon")); createErr != nil {
				return fmt.Errorf("create DatadogAgentProfile %s: %w", profile.Name, createErr)
			}
			continue
		}
		if getErr != nil {
			return fmt.Errorf("read DatadogAgentProfile %s: %w", profile.Name, getErr)
		}
		if err := d.validateManagedAgentInstallationAgentProfile(current, dda, profile); err != nil {
			return err
		}
	}
	return nil
}

func (d *Daemon) managedAgentInstallationAgentProfile(dda *v2alpha1.DatadogAgent, profile decodedManagedAgentInstallationAgentProfile) *v1alpha1.DatadogAgentProfile {
	profileKey := d.managedAgentInstallationKey(profile.Name)
	return &v1alpha1.DatadogAgentProfile{
		TypeMeta: metav1.TypeMeta{
			APIVersion: v1alpha1.GroupVersion.String(),
			Kind:       "DatadogAgentProfile",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: profileKey.Namespace,
			Name:      profileKey.Name,
			Labels: map[string]string{
				fleetManagedByLabel:                        fleetManagedByValue,
				fleetManagedAgentInstallationProviderLabel: string(d.managedAgentInstallationIdentity.Provider()),
				fleetInstallationIDLabel:                   d.managedAgentInstallationIdentity.InstallationID(),
				fleetTargetIDLabel:                         d.managedAgentInstallationIdentity.TargetID(),
			},
			OwnerReferences: []metav1.OwnerReference{controllerOwnerReference(
				v2alpha1.GroupVersion.String(), "DatadogAgent", dda.Name, dda.UID,
			)},
		},
		Spec: v1alpha1.DatadogAgentProfileSpec{
			ProfileAffinity: profile.ProfileAffinity,
			Config:          profile.Config,
		},
	}
}

func (d *Daemon) validateManagedAgentInstallationAgentProfile(profile *v1alpha1.DatadogAgentProfile, dda *v2alpha1.DatadogAgent, wanted decodedManagedAgentInstallationAgentProfile) error {
	if profile.DeletionTimestamp != nil {
		return &stateDoesntMatchError{msg: fmt.Sprintf("DatadogAgentProfile %s/%s is terminating", profile.Namespace, profile.Name)}
	}
	if profile.Labels[fleetManagedByLabel] != fleetManagedByValue ||
		profile.Labels[fleetManagedAgentInstallationProviderLabel] != string(d.managedAgentInstallationIdentity.Provider()) ||
		profile.Labels[fleetInstallationIDLabel] != d.managedAgentInstallationIdentity.InstallationID() ||
		profile.Labels[fleetTargetIDLabel] != d.managedAgentInstallationIdentity.TargetID() {
		return &stateDoesntMatchError{msg: fmt.Sprintf("DatadogAgentProfile %s/%s has invalid managed Agent installation ownership", profile.Namespace, profile.Name)}
	}
	wantOwner := controllerOwnerReference(v2alpha1.GroupVersion.String(), "DatadogAgent", dda.Name, dda.UID)
	if err := requireManagedAgentInstallationResourceOwner(profile.OwnerReferences, wantOwner); err != nil {
		return &stateDoesntMatchError{msg: fmt.Sprintf("DatadogAgentProfile %s/%s is not owned by DatadogAgent %s/%s", profile.Namespace, profile.Name, dda.Namespace, dda.Name)}
	}
	if !apiequality.Semantic.DeepEqual(profile.Spec.ProfileAffinity, wanted.ProfileAffinity) || !apiequality.Semantic.DeepEqual(profile.Spec.Config, wanted.Config) {
		return &stateDoesntMatchError{msg: fmt.Sprintf("DatadogAgentProfile %s/%s differs from the pinned managed Agent installation configuration", profile.Namespace, profile.Name)}
	}
	return nil
}

func (d *Daemon) validateManagedAgentInstallationAgentProfilesExist(ctx context.Context, dda *v2alpha1.DatadogAgent, profiles []decodedManagedAgentInstallationAgentProfile) error {
	for _, profile := range profiles {
		current := &v1alpha1.DatadogAgentProfile{}
		profileKey := d.managedAgentInstallationKey(profile.Name)
		if err := d.managedAgentInstallationReader().Get(ctx, profileKey, current); err != nil {
			return fmt.Errorf("read DatadogAgentProfile %s: %w", profile.Name, err)
		}
		if err := d.validateManagedAgentInstallationAgentProfile(current, dda, profile); err != nil {
			return err
		}
	}
	return nil
}
