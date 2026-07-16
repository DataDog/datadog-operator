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

const addonLifecycleWindowsProfileName = "datadog-agent-windows"

var addonLifecycleWindowsProfileKey = types.NamespacedName{
	Namespace: fleetDatadogAgentNamespace,
	Name:      addonLifecycleWindowsProfileName,
}

func (d *Daemon) ensureAddonLifecycleResources(ctx context.Context, req remoteAPIRequest, dda *v2alpha1.DatadogAgent) error {
	if req.Addon == nil {
		return nil
	}
	wanted := d.addonLifecycleWindowsProfile(dda)
	current := &v1alpha1.DatadogAgentProfile{}
	err := d.lifecycleReader().Get(ctx, addonLifecycleWindowsProfileKey, current)
	if apierrors.IsNotFound(err) {
		if err := d.client.Create(ctx, wanted, client.FieldOwner("fleet-daemon")); err != nil {
			return fmt.Errorf("create Windows DatadogAgentProfile: %w", err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("read Windows DatadogAgentProfile: %w", err)
	}
	return d.validateAddonLifecycleWindowsProfile(current, dda)
}

func (d *Daemon) addonLifecycleWindowsProfile(dda *v2alpha1.DatadogAgent) *v1alpha1.DatadogAgentProfile {
	return &v1alpha1.DatadogAgentProfile{
		TypeMeta: metav1.TypeMeta{
			APIVersion: v1alpha1.GroupVersion.String(),
			Kind:       "DatadogAgentProfile",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: addonLifecycleWindowsProfileKey.Namespace,
			Name:      addonLifecycleWindowsProfileKey.Name,
			Labels: map[string]string{
				fleetManagedByLabel:      fleetManagedByValue,
				fleetInstallationIDLabel: d.lifecycleIdentity.InstallationID,
				fleetTargetIDLabel:       d.lifecycleIdentity.TargetID(),
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

func (d *Daemon) validateAddonLifecycleWindowsProfile(profile *v1alpha1.DatadogAgentProfile, dda *v2alpha1.DatadogAgent) error {
	wanted := d.addonLifecycleWindowsProfile(dda)
	if profile.DeletionTimestamp != nil {
		return &stateDoesntMatchError{msg: fmt.Sprintf("Windows DatadogAgentProfile %s/%s is terminating", profile.Namespace, profile.Name)}
	}
	if profile.Labels[fleetManagedByLabel] != fleetManagedByValue ||
		profile.Labels[fleetInstallationIDLabel] != d.lifecycleIdentity.InstallationID ||
		profile.Labels[fleetTargetIDLabel] != d.lifecycleIdentity.TargetID() {
		return &stateDoesntMatchError{msg: fmt.Sprintf("Windows DatadogAgentProfile %s/%s has invalid lifecycle ownership", profile.Namespace, profile.Name)}
	}
	if err := requireAddonLifecycleWindowsProfileOwner(profile.OwnerReferences, wanted.OwnerReferences[0], dda.UID != ""); err != nil {
		return &stateDoesntMatchError{msg: fmt.Sprintf("Windows DatadogAgentProfile %s/%s is not owned by DatadogAgent %s/%s", profile.Namespace, profile.Name, dda.Namespace, dda.Name)}
	}
	if profile.Annotations[kubernetes.ProviderAnnotationKey] != kubernetes.WindowsProvider || !apiequality.Semantic.DeepEqual(profile.Spec, wanted.Spec) {
		return &stateDoesntMatchError{msg: fmt.Sprintf("Windows DatadogAgentProfile %s/%s differs from the pinned lifecycle configuration", profile.Namespace, profile.Name)}
	}
	return nil
}

func requireAddonLifecycleWindowsProfileOwner(owners []metav1.OwnerReference, want metav1.OwnerReference, requireUID bool) error {
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

func (d *Daemon) waitForAddonLifecycleResourcesReady(ctx context.Context, req remoteAPIRequest, nsn types.NamespacedName, uid types.UID) error {
	if req.Addon == nil {
		return d.waitForFleetDatadogAgentReady(ctx, nsn, uid)
	}
	lastObservation := "waiting for the DatadogAgent and Windows profile controllers"
	err := wait.PollUntilContextTimeout(ctx, lifecycleReadinessPollInterval, lifecycleOperationTimeout, true, func(ctx context.Context) (bool, error) {
		dda := &v2alpha1.DatadogAgent{}
		if err := d.lifecycleReader().Get(ctx, nsn, dda); err != nil {
			return false, err
		}
		ready, observation, err := fleetDatadogAgentReadiness(dda, uid)
		if err != nil || !ready {
			if observation != "" {
				lastObservation = observation
			}
			return false, err
		}
		if err := d.validateAddonLifecycleResourcesReady(ctx, dda); err != nil {
			lastObservation = err.Error()
			return false, nil
		}
		return true, nil
	})
	if err != nil {
		return fmt.Errorf("waiting for add-on lifecycle resources for DatadogAgent %s/%s (%s): %w", nsn.Namespace, nsn.Name, lastObservation, err)
	}
	return nil
}

func (d *Daemon) validateAddonLifecycleResourcesReady(ctx context.Context, dda *v2alpha1.DatadogAgent) error {
	profile := &v1alpha1.DatadogAgentProfile{}
	if err := d.lifecycleReader().Get(ctx, addonLifecycleWindowsProfileKey, profile); err != nil {
		return fmt.Errorf("read Windows DatadogAgentProfile readiness: %w", err)
	}
	if err := d.validateAddonLifecycleWindowsProfile(profile, dda); err != nil {
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

	windowsDaemonSetName := agentprofile.DaemonSetName(addonLifecycleWindowsProfileKey, true)
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

func (d *Daemon) deleteAddonLifecycleWindowsProfile(ctx context.Context, dda *v2alpha1.DatadogAgent) error {
	profile := &v1alpha1.DatadogAgentProfile{}
	if err := d.lifecycleReader().Get(ctx, addonLifecycleWindowsProfileKey, profile); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("read Windows DatadogAgentProfile before deletion: %w", err)
	}
	if err := d.validateAddonLifecycleWindowsProfile(profile, dda); err != nil {
		return err
	}
	preconditions := lifecycleDeletePreconditions(profile.UID, profile.ResourceVersion)
	if err := d.client.Delete(ctx, profile, client.Preconditions(preconditions), client.PropagationPolicy(metav1.DeletePropagationForeground)); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("delete Windows DatadogAgentProfile: %w", err)
	}
	return nil
}

func (d *Daemon) verifyAddonLifecycleWindowsProfileAbsent(ctx context.Context) error {
	profile := &v1alpha1.DatadogAgentProfile{}
	if err := d.lifecycleReader().Get(ctx, addonLifecycleWindowsProfileKey, profile); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("read Windows DatadogAgentProfile after deletion: %w", err)
	}
	return &stateDoesntMatchError{msg: fmt.Sprintf("Windows DatadogAgentProfile %s/%s remains after uninstall", profile.Namespace, profile.Name)}
}

func (d *Daemon) waitForAddonLifecycleWindowsProfileAbsent(ctx context.Context) error {
	return wait.PollUntilContextTimeout(ctx, lifecycleDeletePollInterval, lifecycleOperationTimeout, true, func(ctx context.Context) (bool, error) {
		profile := &v1alpha1.DatadogAgentProfile{}
		err := d.lifecycleReader().Get(ctx, addonLifecycleWindowsProfileKey, profile)
		if apierrors.IsNotFound(err) {
			return true, nil
		}
		if err != nil {
			return false, err
		}
		return false, nil
	})
}
