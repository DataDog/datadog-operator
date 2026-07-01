// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagentinternal

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/merger"
	"github.com/DataDog/datadog-operator/pkg/agentprofile"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

// Shared node Agent local Service reconciliation via per-DDAI port claims.
// The default DDAI owns the Service and merges every DDAI's claims into
// Spec.Ports; profile DDAIs only patch their own claim key. See
// docs/shared_local_service_port_claims.md.

// collectLocalServicePortClaims returns the local-Service ports each enabled
// feature claims, keyed by feature ID (only LocalServicePortClaimer features).
func collectLocalServicePortClaims(features []feature.Feature) map[string][]corev1.ServicePort {
	claims := map[string][]corev1.ServicePort{}
	for _, feat := range features {
		claimer, ok := feat.(feature.LocalServicePortClaimer)
		if !ok {
			continue
		}
		if ports := claimer.LocalServicePortClaim(); len(ports) > 0 {
			claims[string(feat.ID())] = ports
		}
	}
	return claims
}

// shouldCreateLocalService mirrors the version/force gate the APM feature used
// before the port-claim refactor.
func (r *Reconciler) shouldCreateLocalService(spec *v2alpha1.DatadogAgentSpec) bool {
	force := false
	if spec != nil && spec.Global != nil && spec.Global.LocalService != nil {
		force = apiutils.BoolValue(spec.Global.LocalService.ForceEnableLocalService)
	}
	return common.ShouldCreateAgentLocalService(r.platformInfo.GetVersionInfo(), force)
}

// localServiceName resolves the shared Service name for a DDAI. Every DDAI of a
// given DatadogAgent (default and profiles) must resolve to the same name, so it
// is derived from the DatadogAgent name carried on the DDAI label.
func localServiceName(instance *datadoghqv1alpha1.DatadogAgentInternal) string {
	return constants.GetLocalAgentServiceName(constants.GetDDAName(instance), &instance.Spec)
}

// reconcileLocalAgentServiceStore is run for the default (owner) DDAI. It builds
// the shared Service in the dependency store, merging this DDAI's own port
// claims with the claims already published by profile DDAIs (read from the live
// Service annotations). The store applies the result, so the default DDAI is the
// single writer of Spec.Ports. A port conflict between claims is surfaced on the
// offending profile's status and fails the reconcile.
func (r *Reconciler) reconcileLocalAgentServiceStore(ctx context.Context, instance *datadoghqv1alpha1.DatadogAgentInternal, features []feature.Feature, managers feature.ResourceManagers, now metav1.Time) error {
	if !r.shouldCreateLocalService(&instance.Spec) {
		return nil
	}

	name := localServiceName(instance)
	namespace := instance.Namespace

	// Claims already published on the live Service by profile DDAIs.
	liveClaims, err := r.livePortClaimAnnotations(ctx, namespace, name)
	if err != nil {
		return err
	}

	claims := collectLocalServicePortClaims(features)

	obj, found := managers.Store().GetOrCreate(kubernetes.ServicesKind, namespace, name)
	service, ok := obj.(*corev1.Service)
	if !ok {
		return fmt.Errorf("unable to get from the store the Service %s", name)
	}

	// Nothing claims the Service and it is not otherwise in the store: don't
	// create an empty one.
	if !found && len(claims) == 0 && len(liveClaims) == 0 {
		return nil
	}

	service.Spec.Type = corev1.ServiceTypeClusterIP
	service.Spec.Selector = common.GetAgentLocalServiceSelector(instance)
	localPolicy := corev1.ServiceInternalTrafficPolicyLocal
	service.Spec.InternalTrafficPolicy = &localPolicy

	if service.Annotations == nil {
		service.Annotations = map[string]string{}
	}
	// Preserve profile claims, then (re)write our own feature claims.
	maps.Copy(service.Annotations, liveClaims)
	for featureID, ports := range claims {
		encoded, encErr := merger.EncodeServicePorts(ports)
		if encErr != nil {
			return encErr
		}
		service.Annotations[merger.PortClaimAnnotationKey(instance.Name, featureID)] = encoded
	}

	// Merge every port claim and union them with ports other features already
	// added to the store Service directly (e.g. DogStatsD).
	claimedPorts, conflict := merger.MergePortClaims(service.Annotations)
	r.syncPortClaimConflictStatus(ctx, namespace, instance.Name, service.Annotations, conflict, now)
	if conflict != nil {
		return conflict
	}
	mergedPorts, err := merger.MergeServicePorts(service.Spec.Ports, claimedPorts)
	if err != nil {
		return err
	}
	service.Spec.Ports = mergedPorts

	return managers.Store().AddOrUpdate(kubernetes.ServicesKind, service)
}

// publishProfileLocalServicePortClaim is run for profile DDAIs, which do not
// manage dependencies through a store. It patches only this DDAI's own port-claim
// annotation keys onto the Service (a JSON merge patch touching nothing else),
// leaving the default DDAI to merge ports. If the Service does not exist yet, it
// converges on a later reconcile once the default DDAI has created it.
func (r *Reconciler) publishProfileLocalServicePortClaim(ctx context.Context, instance *datadoghqv1alpha1.DatadogAgentInternal, features []feature.Feature) error {
	if !r.shouldCreateLocalService(&instance.Spec) {
		return nil
	}
	claims := collectLocalServicePortClaims(features)
	if len(claims) == 0 {
		return nil
	}

	name := localServiceName(instance)
	service := &corev1.Service{}
	if err := r.client.Get(ctx, types.NamespacedName{Namespace: instance.Namespace, Name: name}, service); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}

	annotations := map[string]string{}
	for featureID, ports := range claims {
		encoded, err := merger.EncodeServicePorts(ports)
		if err != nil {
			return err
		}
		annotations[merger.PortClaimAnnotationKey(instance.Name, featureID)] = encoded
	}

	patch, err := json.Marshal(map[string]any{"metadata": map[string]any{"annotations": annotations}})
	if err != nil {
		return err
	}
	return r.client.Patch(ctx, service, client.RawPatch(types.MergePatchType, patch))
}

// removeLocalServicePortClaim deletes every port-claim annotation this DDAI
// published from the shared Service, so its ports stop being merged once it is
// deleted. The default DDAI re-reads live claims each reconcile, so dropping the
// key here is sufficient — no port pruning is needed elsewhere.
func (r *Reconciler) removeLocalServicePortClaim(ctx context.Context, instance *datadoghqv1alpha1.DatadogAgentInternal) error {
	name := localServiceName(instance)
	service := &corev1.Service{}
	if err := r.client.Get(ctx, types.NamespacedName{Namespace: instance.Namespace, Name: name}, service); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}

	ownPrefix := merger.PortClaimAnnotationPrefix + instance.Name + "."
	nulls := map[string]any{}
	for key := range service.Annotations {
		if strings.HasPrefix(key, ownPrefix) {
			nulls[key] = nil // JSON merge patch: null deletes the key
		}
	}
	if len(nulls) == 0 {
		return nil
	}

	patch, err := json.Marshal(map[string]any{"metadata": map[string]any{"annotations": nulls}})
	if err != nil {
		return err
	}
	return r.client.Patch(ctx, service, client.RawPatch(types.MergePatchType, patch))
}

// livePortClaimAnnotations returns the port-claim.* annotations currently set on
// the live Service, or an empty map if the Service does not exist yet.
func (r *Reconciler) livePortClaimAnnotations(ctx context.Context, namespace, name string) (map[string]string, error) {
	service := &corev1.Service{}
	if err := r.client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, service); err != nil {
		if apierrors.IsNotFound(err) {
			return map[string]string{}, nil
		}
		return nil, err
	}
	out := map[string]string{}
	for key, value := range service.Annotations {
		if strings.HasPrefix(key, merger.PortClaimAnnotationPrefix) {
			out[key] = value
		}
	}
	return out, nil
}

// syncPortClaimConflictStatus reconciles the ServicePortConflict condition on
// every profile that has a port claim: True for the claimant whose claim
// conflicts, False for the rest (so a resolved conflict clears). The default
// DDAI (the DatadogAgent itself) has no profile and is skipped. This uses a
// dedicated condition type so it does not contend with the Applied condition
// that the DDA controller's profile reconciliation manages.
func (r *Reconciler) syncPortClaimConflictStatus(ctx context.Context, namespace, defaultDDAIName string, annotations map[string]string, conflict *merger.PortClaimConflictError, now metav1.Time) {
	for _, name := range merger.ClaimantDDAINames(annotations) {
		if name == defaultDDAIName {
			continue
		}
		message := ""
		if conflict != nil && conflict.DDAIName == name {
			message = conflict.Error()
		}
		r.setProfilePortConflictCondition(ctx, namespace, name, message, now)
	}
}

// setProfilePortConflictCondition best-effort sets the profile's
// ServicePortConflict condition: True with the given message when message is
// non-empty, False otherwise.
func (r *Reconciler) setProfilePortConflictCondition(ctx context.Context, namespace, profileName, message string, now metav1.Time) {
	logger := ctrl.LoggerFrom(ctx)
	profile := &datadoghqv1alpha1.DatadogAgentProfile{}
	if err := r.client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: profileName}, profile); err != nil {
		return // best effort; a claimant without a matching profile is skipped
	}

	status, reason := metav1.ConditionFalse, agentprofile.NoConflictConditionReason
	if message != "" {
		status, reason = metav1.ConditionTrue, agentprofile.ConflictConditionReason
	}

	updated := profile.DeepCopy()
	updated.Status.Conditions = agentprofile.SetDatadogAgentProfileCondition(
		updated.Status.Conditions,
		agentprofile.NewDatadogAgentProfileCondition(agentprofile.ServicePortConflictConditionType, status, now, reason, message),
	)
	if !agentprofile.IsEqualStatus(&profile.Status, &updated.Status) {
		if err := r.client.Status().Update(ctx, updated); err != nil {
			logger.Error(err, "unable to update profile ServicePortConflict status", "datadogagentprofile", profileName)
		}
	}
}
