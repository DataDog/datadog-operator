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
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	v2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/pkg/agentprofile"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

const managedAgentInstallationWindowsProfileName = "datadog-agent-windows"

var managedAgentInstallationWindowsProfileKey = types.NamespacedName{
	Namespace: fleetDatadogAgentNamespace,
	Name:      managedAgentInstallationWindowsProfileName,
}

func (d *Daemon) ensureManagedAgentInstallationWindowsProfile(ctx context.Context, command managedAgentInstallationCommand, dda *v2alpha1.DatadogAgent) error {
	if !command.instrumenterManaged() {
		return nil
	}
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

func (d *Daemon) waitForManagedAgentInstallationReady(ctx context.Context, command managedAgentInstallationCommand, nsn types.NamespacedName, uid types.UID) error {
	if !command.instrumenterManaged() {
		return d.waitForFleetDatadogAgentReady(ctx, nsn, uid)
	}
	lastObservation := "waiting for the DatadogAgent and Windows profile controllers"
	err := wait.PollUntilContextTimeout(ctx, managedAgentInstallationReadinessPollInterval, managedAgentInstallationOperationTimeout, true, func(ctx context.Context) (bool, error) {
		dda := &v2alpha1.DatadogAgent{}
		if err := d.managedAgentInstallationReader().Get(ctx, nsn, dda); err != nil {
			return false, err
		}
		ready, observation, err := fleetDatadogAgentReadiness(dda, uid)
		if err != nil || !ready {
			if observation != "" {
				lastObservation = observation
			}
			return false, err
		}
		resourcesReady, observation := d.managedAgentInstallationWindowsProfileReadiness(ctx, dda)
		if !resourcesReady {
			lastObservation = observation
			return false, nil
		}
		return true, nil
	})
	if err != nil {
		return fmt.Errorf("waiting for managed Agent installation resources for DatadogAgent %s/%s (%s): %w", nsn.Namespace, nsn.Name, lastObservation, err)
	}
	return nil
}

func (d *Daemon) managedAgentInstallationWindowsProfileReadiness(ctx context.Context, dda *v2alpha1.DatadogAgent) (bool, string) {
	if err := d.validateManagedAgentInstallationWindowsProfileReady(ctx, dda); err != nil {
		return false, err.Error()
	}
	return true, ""
}

func (d *Daemon) validateManagedAgentInstallationWindowsProfileReady(ctx context.Context, dda *v2alpha1.DatadogAgent) error {
	profile := &v1alpha1.DatadogAgentProfile{}
	if err := d.managedAgentInstallationReader().Get(ctx, managedAgentInstallationWindowsProfileKey, profile); err != nil {
		return fmt.Errorf("read Windows DatadogAgentProfile readiness: %w", err)
	}
	if err := d.validateManagedAgentInstallationWindowsProfile(profile, dda); err != nil {
		return err
	}
	if profile.Status.Valid != metav1.ConditionTrue || profile.Status.Applied != metav1.ConditionTrue {
		return fmt.Errorf("Windows DatadogAgentProfile has not been validated and applied")
	}
	if profile.Status.CreateStrategy == nil || profile.Status.CreateStrategy.Status != v1alpha1.CompletedStatus {
		return fmt.Errorf("Windows DatadogAgentProfile create strategy is not complete")
	}
	if profile.Status.CreateStrategy.PodsReady != profile.Status.CreateStrategy.NodesLabeled {
		return fmt.Errorf("Windows DatadogAgentProfile has %d ready pods for %d labeled nodes", profile.Status.CreateStrategy.PodsReady, profile.Status.CreateStrategy.NodesLabeled)
	}

	windowsDaemonSetName := agentprofile.DaemonSetName(managedAgentInstallationWindowsProfileKey, true)
	for _, status := range dda.Status.AgentList {
		if status == nil || status.DaemonsetName != windowsDaemonSetName {
			continue
		}
		if status.Desired != profile.Status.CreateStrategy.NodesLabeled {
			return fmt.Errorf("Windows Agent DaemonSet desires %d pods for %d labeled nodes", status.Desired, profile.Status.CreateStrategy.NodesLabeled)
		}
		if !daemonSetStatusReady(status) {
			return fmt.Errorf("Windows Agent DaemonSet is not ready")
		}
		return nil
	}
	return fmt.Errorf("Windows Agent DaemonSet status is missing")
}

func (d *Daemon) deleteManagedAgentInstallationWindowsProfile(ctx context.Context, dda *v2alpha1.DatadogAgent) error {
	profile := &v1alpha1.DatadogAgentProfile{}
	if err := d.managedAgentInstallationReader().Get(ctx, managedAgentInstallationWindowsProfileKey, profile); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("read Windows DatadogAgentProfile before deletion: %w", err)
	}
	if err := d.validateManagedAgentInstallationWindowsProfile(profile, dda); err != nil {
		return err
	}
	preconditions := managedAgentInstallationDeletePreconditions(profile.UID, profile.ResourceVersion)
	if err := d.client.Delete(ctx, profile, client.Preconditions(preconditions), client.PropagationPolicy(metav1.DeletePropagationForeground)); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("delete Windows DatadogAgentProfile: %w", err)
	}
	return nil
}

func (d *Daemon) verifyManagedAgentInstallationWindowsProfileAbsent(ctx context.Context) error {
	profile := &v1alpha1.DatadogAgentProfile{}
	if err := d.managedAgentInstallationReader().Get(ctx, managedAgentInstallationWindowsProfileKey, profile); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("read Windows DatadogAgentProfile after deletion: %w", err)
	}
	return &stateDoesntMatchError{msg: fmt.Sprintf("Windows DatadogAgentProfile %s/%s remains after uninstall", profile.Namespace, profile.Name)}
}

func (d *Daemon) waitForManagedAgentInstallationWindowsProfileAbsent(ctx context.Context) error {
	return wait.PollUntilContextTimeout(ctx, managedAgentInstallationDeletePollInterval, managedAgentInstallationOperationTimeout, true, func(ctx context.Context) (bool, error) {
		profile := &v1alpha1.DatadogAgentProfile{}
		err := d.managedAgentInstallationReader().Get(ctx, managedAgentInstallationWindowsProfileKey, profile)
		if apierrors.IsNotFound(err) {
			return true, nil
		}
		if err != nil {
			return false, err
		}
		return false, nil
	})
}
