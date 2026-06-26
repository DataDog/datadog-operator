// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package remoteconfig

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	kubeclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
)

// RemoteConfigProfileLabelKey marks DatadogAgentProfiles created by the remote
// config updater so they can be reconciled and garbage-collected as a set.
const RemoteConfigProfileLabelKey = "remoteconfig.datadoghq.com/managed"

// reconcileNodeScopedProfiles converts node-scoped remote config updates into
// DatadogAgentProfiles. It creates or updates a profile for each config that
// enables a module and deletes any previously managed profile that is no longer
// desired, so a removed or disabled config tears its profile down.
//
// This assumes a single module (Dynamic Instrumentation) per node selector.
// Adding another node-scoped module means merging every enabled module's
// features into one profile per selector, because profiles are mutually
// exclusive per node and overlapping selectors would otherwise conflict.
func (r *RemoteConfigUpdater) reconcileNodeScopedProfiles(ctx context.Context, configs []DatadogAgentRemoteConfig) error {
	desiredNames := make(map[string]struct{})
	var enabled []DatadogAgentRemoteConfig
	for _, cfg := range configs {
		if cfg.SystemProbe == nil || cfg.SystemProbe.DynamicInstrumentation == nil || !apiutils.BoolValue(cfg.SystemProbe.DynamicInstrumentation.Enabled) {
			continue
		}
		enabled = append(enabled, cfg)
		desiredNames[profileNameFromNodeSelector(cfg.NodeSelector)] = struct{}{}
	}

	if len(enabled) > 0 {
		namespace, err := r.datadogAgentNamespace(ctx)
		if err != nil {
			return err
		}
		for _, cfg := range enabled {
			profile := buildDynamicInstrumentationProfile(profileNameFromNodeSelector(cfg.NodeSelector), namespace, cfg.NodeSelector)
			if err := r.upsertProfile(ctx, profile); err != nil {
				return err
			}
		}
	}

	return r.deleteStaleProfiles(ctx, desiredNames)
}

func (r *RemoteConfigUpdater) datadogAgentNamespace(ctx context.Context) (string, error) {
	ddaList := &v2alpha1.DatadogAgentList{}
	if err := r.kubeClient.List(ctx, ddaList); err != nil {
		return "", fmt.Errorf("unable to list DatadogAgents: %w", err)
	}
	if len(ddaList.Items) == 0 {
		return "", errors.New("cannot find any DatadogAgent")
	}
	return ddaList.Items[0].Namespace, nil
}

func (r *RemoteConfigUpdater) upsertProfile(ctx context.Context, desired *v1alpha1.DatadogAgentProfile) error {
	existing := &v1alpha1.DatadogAgentProfile{}
	getErr := r.kubeClient.Get(ctx, kubeclient.ObjectKeyFromObject(desired), existing)
	if apierrors.IsNotFound(getErr) {
		if err := r.kubeClient.Create(ctx, desired); err != nil {
			return fmt.Errorf("unable to create DatadogAgentProfile %s/%s: %w", desired.Namespace, desired.Name, err)
		}
		r.logger.Info("Created remote-config DatadogAgentProfile", "profile", desired.Name, "namespace", desired.Namespace)
		return nil
	}
	if getErr != nil {
		return fmt.Errorf("unable to get DatadogAgentProfile %s/%s: %w", desired.Namespace, desired.Name, getErr)
	}

	if apiequality.Semantic.DeepEqual(existing.Spec, desired.Spec) {
		return nil
	}

	existing.Spec = desired.Spec
	if err := r.kubeClient.Update(ctx, existing); err != nil {
		return fmt.Errorf("unable to update DatadogAgentProfile %s/%s: %w", desired.Namespace, desired.Name, err)
	}
	r.logger.Info("Updated remote-config DatadogAgentProfile", "profile", desired.Name, "namespace", desired.Namespace)
	return nil
}

func (r *RemoteConfigUpdater) deleteStaleProfiles(ctx context.Context, desiredNames map[string]struct{}) error {
	managed := &v1alpha1.DatadogAgentProfileList{}
	if err := r.kubeClient.List(ctx, managed, kubeclient.MatchingLabels{RemoteConfigProfileLabelKey: "true"}); err != nil {
		return fmt.Errorf("unable to list DatadogAgentProfiles: %w", err)
	}
	for i := range managed.Items {
		profile := &managed.Items[i]
		if _, ok := desiredNames[profile.Name]; ok {
			continue
		}
		if err := r.kubeClient.Delete(ctx, profile); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("unable to delete DatadogAgentProfile %s/%s: %w", profile.Namespace, profile.Name, err)
		}
		r.logger.Info("Deleted stale remote-config DatadogAgentProfile", "profile", profile.Name, "namespace", profile.Namespace)
	}
	return nil
}

func buildDynamicInstrumentationProfile(name, namespace string, nodeSelector []corev1.NodeSelectorRequirement) *v1alpha1.DatadogAgentProfile {
	return &v1alpha1.DatadogAgentProfile{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    map[string]string{RemoteConfigProfileLabelKey: "true"},
		},
		Spec: v1alpha1.DatadogAgentProfileSpec{
			ProfileAffinity: &v1alpha1.ProfileAffinity{
				ProfileNodeAffinity: nodeSelector,
			},
			Config: &v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{
					DynamicInstrumentation: &v2alpha1.DynamicInstrumentationFeatureConfig{
						Enabled: ptr.To(true),
					},
				},
			},
		},
	}
}

// profileNameFromNodeSelector derives a stable, RFC1123-compliant profile name
// from the node selector so the same selection always maps to the same profile.
func profileNameFromNodeSelector(nodeSelector []corev1.NodeSelectorRequirement) string {
	raw, _ := json.Marshal(nodeSelector)
	sum := sha256.Sum256(raw)
	return "datadog-rc-dynamic-instrumentation-" + hex.EncodeToString(sum[:8])
}
