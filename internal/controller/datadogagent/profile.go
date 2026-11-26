// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagent

import (
	"context"
	"encoding/binary"
	"fmt"
	"hash"
	"hash/fnv"
	"io"
	"maps"
	"slices"

	"github.com/prometheus/client_golang/prometheus"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/structured-merge-diff/v6/typed"

	v1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	v2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/experimental"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	featureutils "github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/utils"
	"github.com/DataDog/datadog-operator/internal/controller/metrics"
	"github.com/DataDog/datadog-operator/pkg/agentprofile"
	"github.com/DataDog/datadog-operator/pkg/condition"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/comparison"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
	"github.com/DataDog/datadog-operator/pkg/trace"
)

func sendProfileEnabledMetric(enabled bool) {
	if enabled {
		metrics.DAPEnabled.Set(metrics.TrueValue)
	} else {
		metrics.DAPEnabled.Set(metrics.FalseValue)
	}
}

// setProfileCondition updates a single condition on the profile in place.
func setProfileCondition(profile *v1alpha1.DatadogAgentProfile, conditionType string, status metav1.ConditionStatus, now metav1.Time, reason, message string) {
	profile.Status.Conditions = agentprofile.SetDatadogAgentProfileCondition(
		profile.Status.Conditions,
		agentprofile.NewDatadogAgentProfileCondition(conditionType, status, now, reason, message),
	)
}

// reconcileProfiles reconciles all profiles
// - returns a list of profiles that should be applied (including the default profile)
// - configures node labels based on the profiles that are applied
// - applies profile status updates in k8s
func (r *Reconciler) reconcileProfiles(ctx context.Context, dsNSName types.NamespacedName, ddaMaxUnavailable intstr.IntOrString, defaultDDAI *v1alpha1.DatadogAgentInternal) (_ []*v1alpha1.DatadogAgentProfile, err error) {
	span, ctx := trace.StartSpan(ctx)
	defer trace.FinishSpan(span, &err)

	logger := ctrl.LoggerFrom(ctx)
	now := metav1.Now()
	// start with the default profile so that on error, at minimum the default profile is applied
	defaultProfile := agentprofile.DefaultProfile()
	appliedProfiles := []*v1alpha1.DatadogAgentProfile{&defaultProfile}
	// accumulatedDefaultSpec config starts as the DDA-generated default and
	// changes with each accepted profile's shared overlay contributions.
	baseDefaultSpec := defaultDDAI.Spec.DeepCopy()
	accumulatedDefaultSpec := defaultDDAI.Spec.DeepCopy()

	profilesList := v1alpha1.DatadogAgentProfileList{}
	if err = r.client.List(ctx, &profilesList, client.InNamespace(dsNSName.Namespace)); err != nil {
		return appliedProfiles, fmt.Errorf("unable to list DatadogAgentProfiles: %w", err)
	}
	sortedProfiles := agentprofile.SortProfiles(profilesList.Items)

	nodeList, err := r.getNodeList(ctx)
	if err != nil {
		return appliedProfiles, fmt.Errorf("unable to get node list: %w", err)
	}

	// profilesByNode maps node name to profile
	// it is pre-populated with the default profile
	profilesByNode := make(map[string]types.NamespacedName, len(nodeList))
	for i := range nodeList {
		profilesByNode[nodeList[i].Name] = types.NamespacedName{Namespace: "", Name: "default"}
	}

	// csInfo holds create strategy data per profile
	csInfo := make(map[types.NamespacedName]*agentprofile.CreateStrategyInfo)

	// During the migration to namespace-scoped node labels, nodes keep the
	// legacy shared label until no DaemonSet selects with it.
	legacyLabelInUse, err := agentprofile.LegacyProfileLabelInUse(ctx, r.client)
	if err != nil {
		return appliedProfiles, fmt.Errorf("unable to check legacy profile label usage: %w", err)
	}

	for i := range sortedProfiles {
		profile := &sortedProfiles[i]
		originalStatus := profile.Status.DeepCopy()
		logger.Info("reconciling profile", "datadogagentprofile", profile.Name, "datadogagentprofile_namespace", profile.Namespace)

		r.addDDAIStatusToProfileStatus(ctx, profile, defaultDDAI.Name, defaultDDAI.Namespace, now)

		requirements, err := agentprofile.ValidateProfileAndReturnRequirements(profile)
		if err != nil {
			metrics.DAPValid.With(prometheus.Labels{"datadogagentprofile": profile.Name}).Set(metrics.FalseValue)
			setProfileCondition(profile, agentprofile.ValidConditionType, metav1.ConditionFalse, now, agentprofile.InvalidConditionReason, err.Error())
			setProfileCondition(profile, agentprofile.AppliedConditionType, metav1.ConditionUnknown, now, agentprofile.InvalidConditionReason, "Profile is invalid")
			logger.Error(err, "unable to reconcile profile", "datadogagentprofile", profile.Name, "datadogagentprofile_namespace", profile.Namespace)
			r.syncProfileStatus(ctx, profile, originalStatus, now)
			continue
		}
		metrics.DAPValid.With(prometheus.Labels{"datadogagentprofile": profile.Name}).Set(metrics.TrueValue)
		setProfileCondition(profile, agentprofile.ValidConditionType, metav1.ConditionTrue, now, agentprofile.ValidConditionReason, "Valid manifest")

		if err := agentprofile.CheckProfileNodeConflicts(profile.ObjectMeta, requirements, nodeList, profilesByNode); err != nil {
			setProfileCondition(profile, agentprofile.AppliedConditionType, metav1.ConditionFalse, now, agentprofile.ConflictConditionReason, "Conflict with existing profile")
			logger.Error(err, "unable to reconcile profile", "datadogagentprofile", profile.Name, "datadogagentprofile_namespace", profile.Namespace)
			r.syncProfileStatus(ctx, profile, originalStatus, now)
			continue
		}

		// Modify default DDAI spec with shared configs, e.g. DCA configs.
		// Spec changes are applied later if there is no error.
		// candidateDefaultSpec = accumulated spec config from default DDAI and profiles.
		// baseDefaultSpec = original default DDAI spec before any profile overlays were applied (used to detect user-configured vs defaulted configs)
		candidateDefaultSpec := accumulatedDefaultSpec.DeepCopy()
		if err := feature.ApplyProfileSharedConfigOverlays(candidateDefaultSpec, baseDefaultSpec, profile.Spec.Config); err != nil {
			setProfileCondition(profile, agentprofile.AppliedConditionType, metav1.ConditionFalse, now, agentprofile.ConflictConditionReason, err.Error())
			logger.Error(err, "unable to reconcile profile", "datadogagentprofile", profile.Name, "datadogagentprofile_namespace", profile.Namespace)
			r.syncProfileStatus(ctx, profile, originalStatus, now)
			continue
		}

		// The profile is valid and its shared overlay was accepted.
		// Commit node assignment and shared overlay changes.
		agentprofile.AssignNodesToProfile(profile.ObjectMeta, requirements, nodeList, profilesByNode, csInfo)
		accumulatedDefaultSpec = candidateDefaultSpec
		setProfileCondition(profile, agentprofile.AppliedConditionType, metav1.ConditionTrue, now, agentprofile.AppliedConditionReason, "Profile applied")
		r.syncProfileStatus(ctx, profile, originalStatus, now)
		appliedProfiles = append(appliedProfiles, profile)
	}

	// Persist the accepted shared config back to the default DDAI used later to
	// render the default-profile DDAI object.
	defaultDDAI.Spec = *accumulatedDefaultSpec

	if err := r.enforceCreateStrategy(ctx, appliedProfiles, profilesByNode, csInfo, dsNSName, ddaMaxUnavailable, len(nodeList)); err != nil {
		return appliedProfiles, err
	}

	r.markCrossNamespaceProfiles(ctx, dsNSName.Namespace, now)

	if err := r.labelNodesWithProfiles(ctx, profilesByNode, dsNSName.Namespace, legacyLabelInUse, nodeList); err != nil {
		return appliedProfiles, fmt.Errorf("unable to label nodes with profiles: %w", err)
	}

	if err := r.cleanupPodsForProfilesThatNoLongerApply(ctx, profilesByNode, dsNSName.Namespace); err != nil {
		return appliedProfiles, fmt.Errorf("unable to cleanup pods for profiles that no longer apply: %w", err)
	}

	return appliedProfiles, nil
}

// markCrossNamespaceProfiles marks profiles outside the DatadogAgent namespace
// as not applied. Profiles only apply to the DatadogAgent in their own
// namespace, so without this, a profile that used to apply across namespaces
// would keep reporting Applied=True after it stopped being used.
func (r *Reconciler) markCrossNamespaceProfiles(ctx context.Context, ddaNamespace string, now metav1.Time) {
	logger := ctrl.LoggerFrom(ctx)

	profilesList := v1alpha1.DatadogAgentProfileList{}
	if err := r.client.List(ctx, &profilesList); err != nil {
		logger.Error(err, "unable to list DatadogAgentProfiles")
		return
	}
	for i := range profilesList.Items {
		profile := &profilesList.Items[i]
		if profile.Namespace == ddaNamespace {
			continue
		}
		originalStatus := profile.Status.DeepCopy()
		setProfileCondition(profile, agentprofile.AppliedConditionType, metav1.ConditionFalse, now, agentprofile.NoDatadogAgentConditionReason,
			"No DatadogAgent in this namespace. DatadogAgentProfiles only apply to DatadogAgents in the same namespace.")
		profile.Status.CreateStrategy = nil
		r.syncProfileStatus(ctx, profile, originalStatus, now)
	}
}

// enforceCreateStrategy updates create-strategy status for applied profiles and
// prunes profilesByNode so only nodes allowed by the strategy are labeled.
func (r *Reconciler) enforceCreateStrategy(ctx context.Context, appliedProfiles []*v1alpha1.DatadogAgentProfile, profilesByNode map[string]types.NamespacedName, csInfo map[types.NamespacedName]*agentprofile.CreateStrategyInfo, dsNSName types.NamespacedName, ddaMaxUnavailable intstr.IntOrString, nodeCount int) error {
	if !agentprofile.CreateStrategyEnabled() {
		return nil
	}
	logger := ctrl.LoggerFrom(ctx)
	for _, profile := range appliedProfiles {
		if !agentprofile.CreateStrategyNeeded(profile, csInfo) {
			continue
		}

		// get profile ds to set create strategy status
		ds, err := r.getProfileDaemonSet(ctx, profile, dsNSName)
		if err != nil {
			return fmt.Errorf("unable to get profile daemon set: %w", err)
		}

		profileCopy := profile.DeepCopy()
		agentprofile.ApplyCreateStrategy(logger, profilesByNode, csInfo[types.NamespacedName{Namespace: profile.Namespace, Name: profile.Name}], profileCopy, ddaMaxUnavailable, nodeCount, &ds.Status)
		if !agentprofile.IsEqualStatus(&profile.Status, &profileCopy.Status) {
			if err := r.client.Status().Update(ctx, profileCopy); err != nil {
				logger.Error(err, "unable to update profile status", "datadogagentprofile", profileCopy.Name, "datadogagentprofile_namespace", profileCopy.Namespace)
			}
		}
	}
	return nil
}

// syncProfileStatus generates status from conditions and persists it to k8s if it changed.
func (r *Reconciler) syncProfileStatus(ctx context.Context, profile *v1alpha1.DatadogAgentProfile, originalStatus *v1alpha1.DatadogAgentProfileStatus, now metav1.Time) {
	logger := ctrl.LoggerFrom(ctx)
	agentprofile.GenerateProfileStatusFromConditions(logger, profile, now)
	if !agentprofile.IsEqualStatus(originalStatus, &profile.Status) {
		if err := r.client.Status().Update(ctx, profile); err != nil {
			logger.Error(err, "unable to update profile status", "datadogagentprofile", profile.Name, "datadogagentprofile_namespace", profile.Namespace)
		}
	}
}

func (r *Reconciler) getProfileDaemonSet(ctx context.Context, profile *v1alpha1.DatadogAgentProfile, dsName types.NamespacedName) (*appsv1.DaemonSet, error) {
	validDaemonSetNames := r.getValidDaemonSetNames(dsName.Name, map[string]struct{}{}, []v1alpha1.DatadogAgentProfile{*profile}, true)
	if len(validDaemonSetNames) != 1 {
		return nil, fmt.Errorf("unexpected number of valid daemonset names: %d", len(validDaemonSetNames))
	}

	for name := range validDaemonSetNames {
		ds := &appsv1.DaemonSet{}
		if err := r.client.Get(ctx, types.NamespacedName{Namespace: dsName.Namespace, Name: name}, ds); err != nil && !apierrors.IsNotFound(err) {
			return nil, err
		}
		return ds, nil
	}
	return nil, fmt.Errorf("no valid daemonset found")
}

type profileMergeCacheKey struct {
	ddaiNamespace string
	ddaiName      string
	profileNS     string
	profileName   string
}

type profileMergeCacheEntry struct {
	fieldManager   *crdFieldManager
	sourceHash     [16]byte
	profileUID     types.UID
	profileRV      string
	profileContent [16]byte
	merged         *v1alpha1.DatadogAgentInternal
}

func writeStringField(h hash.Hash, s string) {
	var lenBuf [8]byte
	binary.LittleEndian.PutUint64(lenBuf[:], uint64(len(s)))
	_, _ = h.Write(lenBuf[:])
	_, _ = io.WriteString(h, s)
}

func writeSortedStringMap(h hash.Hash, m map[string]string) {
	var lenBuf [8]byte
	binary.LittleEndian.PutUint64(lenBuf[:], uint64(len(m)))
	_, _ = h.Write(lenBuf[:])
	if len(m) == 0 {
		return
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for _, k := range keys {
		writeStringField(h, k)
		writeStringField(h, m[k])
	}
}

func fingerprintSourceDDAI(ddai *v1alpha1.DatadogAgentInternal) ([16]byte, bool) {
	var out [16]byte
	if ddai == nil || hasPopulatedManagedFields(ddai) {
		return out, false
	}
	specHash, err := comparison.GenerateMD5ForSpec(&ddai.Spec)
	if err != nil {
		return out, false
	}
	h := fnv.New128a()
	writeStringField(h, ddai.Namespace)
	writeStringField(h, ddai.Name)
	writeStringField(h, specHash)
	writeSortedStringMap(h, ddai.Labels)
	writeSortedStringMap(h, ddai.Annotations)
	var lenBuf [8]byte
	binary.LittleEndian.PutUint64(lenBuf[:], uint64(len(ddai.Finalizers)))
	_, _ = h.Write(lenBuf[:])
	for _, f := range ddai.Finalizers {
		writeStringField(h, f)
	}
	binary.LittleEndian.PutUint64(lenBuf[:], uint64(len(ddai.OwnerReferences)))
	_, _ = h.Write(lenBuf[:])
	for i := range ddai.OwnerReferences {
		ref := &ddai.OwnerReferences[i]
		writeStringField(h, ref.APIVersion)
		writeStringField(h, ref.Kind)
		writeStringField(h, ref.Name)
		writeStringField(h, string(ref.UID))
	}
	h.Sum(out[:0])
	return out, true
}

func fingerprintProfileContent(profile *v1alpha1.DatadogAgentProfile) ([16]byte, bool) {
	var out [16]byte
	if agentprofile.IsDefaultProfile(profile.Namespace, profile.Name) {
		return out, true
	}
	specHash, err := comparison.GenerateMD5ForSpec(&profile.Spec)
	if err != nil {
		return out, false
	}
	h := fnv.New128a()
	writeStringField(h, profile.Namespace)
	writeStringField(h, profile.Name)
	writeStringField(h, specHash)
	writeSortedStringMap(h, profile.Annotations)
	h.Sum(out[:0])
	return out, true
}

func (r *Reconciler) lookupProfileMergeCache(key profileMergeCacheKey, sourceHash [16]byte, profile *v1alpha1.DatadogAgentProfile, profileContent [16]byte, useRV bool) (*v1alpha1.DatadogAgentInternal, bool) {
	r.profileMergeMu.RLock()
	entry, ok := r.profileMergeCache[key]
	r.profileMergeMu.RUnlock()
	if !ok {
		return nil, false
	}
	if entry.fieldManager != r.fieldManager || entry.sourceHash != sourceHash {
		return nil, false
	}
	if useRV {
		if entry.profileUID != profile.UID || entry.profileRV != profile.ResourceVersion {
			return nil, false
		}
	} else if entry.profileContent != profileContent {
		return nil, false
	}
	return entry.merged.DeepCopy(), true
}

func (r *Reconciler) storeProfileMergeCache(key profileMergeCacheKey, entry profileMergeCacheEntry) {
	r.profileMergeMu.Lock()
	if r.profileMergeCache == nil {
		r.profileMergeCache = make(map[profileMergeCacheKey]profileMergeCacheEntry)
	}
	r.profileMergeCache[key] = entry
	r.profileMergeMu.Unlock()
}

func (r *Reconciler) pruneProfileMergeCache(ddaiNamespace, ddaiName string, activeKeys map[profileMergeCacheKey]struct{}) {
	r.profileMergeMu.RLock()
	needsPrune := false
	for k := range r.profileMergeCache {
		if k.ddaiNamespace == ddaiNamespace && k.ddaiName == ddaiName {
			if _, active := activeKeys[k]; !active {
				needsPrune = true
				break
			}
		}
	}
	r.profileMergeMu.RUnlock()
	if !needsPrune {
		return
	}

	r.profileMergeMu.Lock()
	for k := range r.profileMergeCache {
		if k.ddaiNamespace == ddaiNamespace && k.ddaiName == ddaiName {
			if _, active := activeKeys[k]; !active {
				delete(r.profileMergeCache, k)
			}
		}
	}
	r.profileMergeMu.Unlock()
}

func (r *Reconciler) applyProfilesToDDAISpec(ctx context.Context, baseDDAI, defaultDDAI *v1alpha1.DatadogAgentInternal, profiles []*v1alpha1.DatadogAgentProfile) ([]*v1alpha1.DatadogAgentInternal, error) {
	ddais := make([]*v1alpha1.DatadogAgentInternal, 0, len(profiles))

	baseHash, baseCacheable := fingerprintSourceDDAI(baseDDAI)
	defaultHash := baseHash
	defaultCacheable := baseCacheable
	if baseDDAI != defaultDDAI {
		defaultHash, defaultCacheable = fingerprintSourceDDAI(defaultDDAI)
	}

	// Lazily initialize schema validation and pre-converted nodeAgent TypedValue
	// only if at least one profile misses the steady-state merge cache.
	baseSchemaChecked := false
	baseSchemaValid := false
	defaultSchemaChecked := false
	defaultSchemaValid := false
	typedNodeAgentBaseChecked := false
	var typedNodeAgentBase *typed.TypedValue

	activeKeys := make(map[profileMergeCacheKey]struct{}, len(profiles))

	// For all profiles, create DDAI objects
	// Note: profiles includes the default profile to allow the default affinity to be set
	for _, profile := range profiles {
		isDefault := agentprofile.IsDefaultProfile(profile.Namespace, profile.Name)
		// User profiles are merged from the original base DDAI. The synthetic
		// default profile is merged from defaultDDAI so it includes shared config
		// accepted from profiles, for example APM SSI Cluster Agent config.
		sourceDDAI := baseDDAI
		sourceHash := baseHash
		cacheable := baseCacheable
		if isDefault {
			sourceDDAI = defaultDDAI
			sourceHash = defaultHash
			cacheable = defaultCacheable
		}

		cacheKey := profileMergeCacheKey{
			ddaiNamespace: sourceDDAI.Namespace,
			ddaiName:      sourceDDAI.Name,
			profileNS:     profile.Namespace,
			profileName:   profile.Name,
		}
		activeKeys[cacheKey] = struct{}{}

		useRV := !isDefault && profile.UID != "" && profile.ResourceVersion != ""
		var profileContent [16]byte
		if cacheable && !useRV {
			var ok bool
			profileContent, ok = fingerprintProfileContent(profile)
			if !ok {
				cacheable = false
			}
		}

		if cacheable {
			if cached, hit := r.lookupProfileMergeCache(cacheKey, sourceHash, profile, profileContent, useRV); hit {
				ddais = append(ddais, cached)
				continue
			}
		}

		// Cache miss: lazily validate CRD schema and build shared nodeAgent TypedValue once.
		var schemaValid bool
		var preTypedNodeBase *typed.TypedValue
		if isDefault {
			if !defaultSchemaChecked {
				if baseDDAI == defaultDDAI && baseSchemaChecked {
					defaultSchemaValid = baseSchemaValid
				} else {
					defaultSchemaValid = r.isDDAISchemaValid(defaultDDAI)
				}
				defaultSchemaChecked = true
			}
			schemaValid = defaultSchemaValid
		} else {
			if !baseSchemaChecked {
				baseSchemaValid = r.isDDAISchemaValid(baseDDAI)
				baseSchemaChecked = true
			}
			schemaValid = baseSchemaValid
			if schemaValid && !typedNodeAgentBaseChecked && len(profiles) > 1 {
				typedNodeAgentBaseChecked = true
				gvk := getDDAIGVK()
				minNodeBase := &v1alpha1.DatadogAgentInternal{
					ObjectMeta: metav1.ObjectMeta{
						Name:      baseDDAI.Name,
						Namespace: baseDDAI.Namespace,
					},
				}
				minNodeBase.GetObjectKind().SetGroupVersionKind(gvk)
				if baseOv, ok := baseDDAI.Spec.Override[v2alpha1.NodeAgentComponentName]; ok && baseOv != nil {
					minNodeBase.Spec.Override = map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
						v2alpha1.NodeAgentComponentName: baseOv,
					}
				}
				if uMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(minNodeBase); err == nil {
					uObj := &unstructured.Unstructured{Object: uMap}
					uObj.SetGroupVersionKind(gvk)
					if tv, err := r.fieldManager.typeConverter.ObjectToTyped(uObj, typed.AllowDuplicates); err == nil {
						typedNodeAgentBase = tv
					}
				}
			}
			preTypedNodeBase = typedNodeAgentBase
		}

		var mergedDDAI *v1alpha1.DatadogAgentInternal
		var err error
		if isDefault && !defaultSchemaChecked && baseDDAI != defaultDDAI {
			defaultSchemaChecked = true
			mergedDDAI, err = r.computeProfileMerge(ctx, sourceDDAI, profile)
		} else {
			mergedDDAI, err = r.computeProfileMergeInternal(ctx, sourceDDAI, profile, schemaValid, preTypedNodeBase)
		}
		if err != nil {
			return nil, err
		}

		if cacheable {
			entry := profileMergeCacheEntry{
				fieldManager:   r.fieldManager,
				sourceHash:     sourceHash,
				profileContent: profileContent,
				merged:         mergedDDAI.DeepCopy(),
			}
			if useRV {
				entry.profileUID = profile.UID
				entry.profileRV = profile.ResourceVersion
			}
			r.storeProfileMergeCache(cacheKey, entry)
		}

		ddais = append(ddais, mergedDDAI)
	}

	if baseDDAI != nil {
		r.pruneProfileMergeCache(baseDDAI.Namespace, baseDDAI.Name, activeKeys)
	}

	return ddais, nil
}

func (r *Reconciler) isDDAISchemaValid(ddai *v1alpha1.DatadogAgentInternal) bool {
	if r.fieldManager == nil || ddai == nil || hasPopulatedManagedFields(ddai) {
		return false
	}
	withGVK := *ddai
	withGVK.GetObjectKind().SetGroupVersionKind(getDDAIGVK())
	_, err := r.fieldManager.objectToTypedLive(&withGVK)
	return err == nil
}

func (r *Reconciler) computeProfileMerge(ctx context.Context, ddai *v1alpha1.DatadogAgentInternal, profile *v1alpha1.DatadogAgentProfile) (*v1alpha1.DatadogAgentInternal, error) {
	return r.computeProfileMergeInternal(ctx, ddai, profile, r.isDDAISchemaValid(ddai), nil)
}

func (r *Reconciler) computeProfileMergeInternal(ctx context.Context, ddai *v1alpha1.DatadogAgentInternal, profile *v1alpha1.DatadogAgentProfile, baseSchemaValid bool, preTypedNodeBase *typed.TypedValue) (*v1alpha1.DatadogAgentInternal, error) {
	isDefault := agentprofile.IsDefaultProfile(profile.Namespace, profile.Name)

	// Fast path when baseDDAI already conforms to the installed CRD schema:
	// merge only the subtrees actually touched by the profile via SMD, and
	// copy untouched subtrees directly.
	if baseSchemaValid {
		typedObj, err := r.computeProfileMergeSubtree(ctx, ddai, profile, isDefault, preTypedNodeBase)
		if err != nil {
			return nil, err
		}
		if experimental.IsAutopilotEnabled(typedObj) {
			ensureGCRAutopilotRegistry(&typedObj.Spec)
		}
		if _, err := comparison.SetMD5GenerationAnnotation(&typedObj.ObjectMeta, typedObj.Spec, constants.MD5DDAIDeploymentAnnotationKey); err != nil {
			return nil, err
		}
		return typedObj, nil
	}

	// Fallback path (outdated CRD schema or populated ManagedFields):
	// run full-object SSA merge so missing schema fields are detected, stripped, and logged.
	return r.computeProfileMergeFull(ctx, ddai, profile)
}

func (r *Reconciler) computeProfileMergeSubtree(ctx context.Context, ddai *v1alpha1.DatadogAgentInternal, profile *v1alpha1.DatadogAgentProfile, isDefault bool, preTypedNodeBase *typed.TypedValue) (*v1alpha1.DatadogAgentInternal, error) {
	affinity := setProfileDDAIAffinity(ddai, profile)

	// Default profile only sets Override[NodeAgent].Affinity and clears ManagedFields.
	// Since ddai already passed CRD schema validation, we can build the result directly
	// without re-serializing the entire object through structured-merge-diff.
	if isDefault {
		out := ddai.DeepCopy()
		if err := setProfileDDAIMeta(out, profile); err != nil {
			return nil, err
		}
		out.GetObjectKind().SetGroupVersionKind(getDDAIGVK())
		ensureOverrideExists(out, v2alpha1.NodeAgentComponentName)
		out.Spec.Override[v2alpha1.NodeAgentComponentName].Affinity = affinity
		return out, nil
	}

	// Build minimal base and patch objects containing ONLY the Spec subtrees
	// that profile.Spec.Config actually touches, so SMD only walks the diff.
	gvk := getDDAIGVK()
	minBase := &v1alpha1.DatadogAgentInternal{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ddai.Name,
			Namespace: ddai.Namespace,
		},
	}
	minBase.GetObjectKind().SetGroupVersionKind(gvk)

	minPatch := &v1alpha1.DatadogAgentInternal{
		ObjectMeta: metav1.ObjectMeta{
			Name:      getProfileDDAIName(ddai.Name, profile.Name, profile.Namespace),
			Namespace: ddai.Namespace,
		},
	}
	minPatch.GetObjectKind().SetGroupVersionKind(gvk)

	profileCfg := profile.Spec.Config
	mergeGlobal := profileCfg != nil && profileCfg.Global != nil
	mergeFeatures := profileCfg != nil && profileCfg.Features != nil

	if mergeGlobal {
		minBase.Spec.Global = ddai.Spec.Global
		minPatch.Spec.Global = profileCfg.Global.DeepCopy()
		// Preserve base commonLabels when profile also sets Global (profile keys win).
		if ddai.Spec.Global != nil && len(ddai.Spec.Global.CommonLabels) > 0 {
			if minPatch.Spec.Global.CommonLabels == nil {
				minPatch.Spec.Global.CommonLabels = make(map[string]string, len(ddai.Spec.Global.CommonLabels))
			}
			for k, v := range ddai.Spec.Global.CommonLabels {
				if _, exists := minPatch.Spec.Global.CommonLabels[k]; !exists {
					minPatch.Spec.Global.CommonLabels[k] = v
				}
			}
		}
	}

	if mergeFeatures {
		minBase.Spec.Features = ddai.Spec.Features
		minPatch.Spec.Features = profileCfg.Features.DeepCopy()
	}

	// Determine which Override components need SMD merge vs direct copy.
	// NodeAgent is always modified (profile affinity, label, daemonset name).
	// Other components only need SMD merge if profileCfg.Override explicitly configures them.
	autoDisabledComponents := [3]v2alpha1.ComponentName{
		v2alpha1.ClusterAgentComponentName,
		v2alpha1.ClusterChecksRunnerComponentName,
		v2alpha1.OtelAgentGatewayComponentName,
	}

	var mergeComponents []v2alpha1.ComponentName
	mergeComponents = append(mergeComponents, v2alpha1.NodeAgentComponentName)
	if profileCfg != nil {
		for comp := range profileCfg.Override {
			if comp != v2alpha1.NodeAgentComponentName {
				mergeComponents = append(mergeComponents, comp)
			}
		}
	}

	minBase.Spec.Override = make(map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride, len(mergeComponents))
	minPatch.Spec.Override = make(map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride, len(mergeComponents))

	for _, comp := range mergeComponents {
		if baseOv, ok := ddai.Spec.Override[comp]; ok && baseOv != nil {
			minBase.Spec.Override[comp] = baseOv
		}
		if profileCfg != nil && profileCfg.Override[comp] != nil {
			minPatch.Spec.Override[comp] = profileCfg.Override[comp].DeepCopy()
		} else {
			minPatch.Spec.Override[comp] = &v2alpha1.DatadogAgentComponentOverride{}
		}
	}

	// Ensure auto-disabled components in mergeComponents also get Disabled=true in the patch.
	for _, comp := range autoDisabledComponents {
		if ov, ok := minPatch.Spec.Override[comp]; ok {
			ov.Disabled = new(true)
		}
	}

	setProfileNodeAgentOverride(minPatch, profile)
	minPatch.Spec.Override[v2alpha1.NodeAgentComponentName].Affinity = affinity

	// Reuse pre-converted nodeAgent-only TypedValue when only nodeAgent is being merged.
	var preTyped *typed.TypedValue
	if !mergeGlobal && !mergeFeatures && len(mergeComponents) == 1 {
		preTyped = preTypedNodeBase
	}

	// Run schema-aware SMD merge on the minimal subtrees only.
	obj, err := r.ssaMergeCRDWithTypedOriginal(ctx, minBase, minPatch, preTyped)
	if err != nil {
		return nil, err
	}
	typedObj, ok := obj.(*v1alpha1.DatadogAgentInternal)
	if !ok {
		return nil, fmt.Errorf("unexpected type: %T", obj)
	}

	// Reassemble final ObjectMeta from ddai + profile metadata overrides.
	ddai.ObjectMeta.DeepCopyInto(&typedObj.ObjectMeta)
	if err := setProfileDDAIMeta(typedObj, profile); err != nil {
		return nil, err
	}
	typedObj.GetObjectKind().SetGroupVersionKind(gvk)

	// Reattach untouched Global and Features directly via DeepCopy.
	if !mergeGlobal && ddai.Spec.Global != nil {
		typedObj.Spec.Global = ddai.Spec.Global.DeepCopy()
	}
	if !mergeFeatures && ddai.Spec.Features != nil {
		typedObj.Spec.Features = ddai.Spec.Features.DeepCopy()
	}

	// Reattach untouched Override components and ensure auto-disabled components are disabled.
	if typedObj.Spec.Override == nil {
		typedObj.Spec.Override = make(map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride, len(ddai.Spec.Override)+len(autoDisabledComponents))
	}
	for comp, baseOv := range ddai.Spec.Override {
		if _, merged := minPatch.Spec.Override[comp]; !merged {
			typedObj.Spec.Override[comp] = baseOv.DeepCopy()
		}
	}
	for _, comp := range autoDisabledComponents {
		if _, merged := minPatch.Spec.Override[comp]; !merged {
			disableComponent(typedObj, comp)
		}
	}

	return typedObj, nil
}

func (r *Reconciler) computeProfileMergeFull(ctx context.Context, ddai *v1alpha1.DatadogAgentInternal, profile *v1alpha1.DatadogAgentProfile) (*v1alpha1.DatadogAgentInternal, error) {
	// Copy the original DDAI and apply profile spec to create a fake "DDAI" to merge
	profileDDAI := ddai.DeepCopy()
	baseDDAI := ddai.DeepCopy()
	profileCopy := profile.DeepCopy()

	// Add profile settings to "fake" DDAI
	setProfileSpec(profileDDAI, profileCopy)
	if err := setProfileDDAIMeta(profileDDAI, profileCopy); err != nil {
		return nil, err
	}

	// ensure gvk is set
	baseDDAI.GetObjectKind().SetGroupVersionKind(getDDAIGVK())
	profileDDAI.GetObjectKind().SetGroupVersionKind(getDDAIGVK())

	// Server side apply to merge DDAIs
	obj, err := r.ssaMergeCRD(ctx, baseDDAI, profileDDAI)
	if err != nil {
		return nil, err
	}

	typedObj, ok := obj.(*v1alpha1.DatadogAgentInternal)
	if !ok {
		return nil, fmt.Errorf("unexpected type: %T", obj)
	}
	if experimental.IsAutopilotEnabled(typedObj) {
		ensureGCRAutopilotRegistry(&typedObj.Spec)
	}
	if _, err := comparison.SetMD5GenerationAnnotation(&typedObj.ObjectMeta, typedObj.Spec, constants.MD5DDAIDeploymentAnnotationKey); err != nil {
		return nil, err
	}
	return typedObj, nil
}

func setProfileSpec(ddai *v1alpha1.DatadogAgentInternal, profile *v1alpha1.DatadogAgentProfile) {
	// create affinity from ddai and profile prior to re-set after replacing the ddai spec
	affinity := setProfileDDAIAffinity(ddai, profile)
	if !agentprofile.IsDefaultProfile(profile.Namespace, profile.Name) {
		// Capture spec.global.commonLabels from the base DDAI before replacing
		// the spec with the profile config. The profile's Config is a user-defined
		// DatadogAgentSpec that doesn't include the parent DDA's global settings,
		// so commonLabels would be silently dropped. We restore them afterward so
		// that label-enforcing admission policies (e.g. Kyverno) do not reject
		// the profile DaemonSet even when the parent DDA sets spec.global.commonLabels.
		var commonLabels map[string]string
		if ddai.Spec.Global != nil && len(ddai.Spec.Global.CommonLabels) > 0 {
			commonLabels = make(map[string]string, len(ddai.Spec.Global.CommonLabels))
			maps.Copy(commonLabels, ddai.Spec.Global.CommonLabels)
		}

		ddai.Spec = *profile.Spec.Config

		// Restore commonLabels into the replaced spec.
		if len(commonLabels) > 0 {
			if ddai.Spec.Global == nil {
				ddai.Spec.Global = &v2alpha1.GlobalConfig{}
			}
			// Profile config wins on any key conflict — only fill in keys
			// not already set by the profile itself.
			if ddai.Spec.Global.CommonLabels == nil {
				ddai.Spec.Global.CommonLabels = make(map[string]string, len(commonLabels))
			}
			for k, v := range commonLabels {
				if _, exists := ddai.Spec.Global.CommonLabels[k]; !exists {
					ddai.Spec.Global.CommonLabels[k] = v
				}
			}
		}

		// DCA, CCR, and OtelAgentGateway are auto disabled for user created profiles
		disableComponent(ddai, v2alpha1.ClusterAgentComponentName)
		disableComponent(ddai, v2alpha1.ClusterChecksRunnerComponentName)
		disableComponent(ddai, v2alpha1.OtelAgentGatewayComponentName)
		setProfileNodeAgentOverride(ddai, profile)
	}
	ensureOverrideExists(ddai, v2alpha1.NodeAgentComponentName)
	ddai.Spec.Override[v2alpha1.NodeAgentComponentName].Affinity = affinity
}

func ensureOverrideExists(ddai *v1alpha1.DatadogAgentInternal, componentName v2alpha1.ComponentName) {
	if ddai.Spec.Override == nil {
		ddai.Spec.Override = make(map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride)
	}
	if ddai.Spec.Override[componentName] == nil {
		ddai.Spec.Override[componentName] = &v2alpha1.DatadogAgentComponentOverride{}
	}
}

func disableComponent(ddai *v1alpha1.DatadogAgentInternal, componentName v2alpha1.ComponentName) {
	ensureOverrideExists(ddai, componentName)
	ddai.Spec.Override[componentName].Disabled = new(true)
}

func setProfileDDAIAffinity(ddai *v1alpha1.DatadogAgentInternal, profile *v1alpha1.DatadogAgentProfile) *corev1.Affinity {
	override, ok := ddai.Spec.Override[v2alpha1.NodeAgentComponentName]
	if !ok || override == nil {
		override = &v2alpha1.DatadogAgentComponentOverride{}
	}
	return common.MergeAffinities(override.Affinity, agentprofile.AffinityOverride(profile, ddai.Namespace))
}

func setProfileDDAIMeta(ddai *v1alpha1.DatadogAgentInternal, profile *v1alpha1.DatadogAgentProfile) error {
	// Name
	ddai.Name = getProfileDDAIName(ddai.Name, profile.Name, profile.Namespace)
	// Managed fields needs to be nil for server-side apply
	ddai.ManagedFields = nil
	// Include the profile label in the DDAI metadata only for non-default profiles.
	// This distinguishes profile-specific DDAIs and can also support their garbage collection.
	if !agentprofile.IsDefaultProfile(profile.Namespace, profile.Name) {
		// This should not happen, as we add the DDA label upon creation of the DDAI.
		if ddai.Labels == nil {
			ddai.Labels = make(map[string]string)
		}
		ddai.Labels[constants.ProfileLabelKey] = profile.Name
	}
	// Propagate the provider annotation from the profile onto the DDAI so a
	// DAP can declare a provider that differs from the DDA (e.g. a GKE COS
	// node pool selected by the profile). The profile value overrides the
	// DDA-inherited value when set.
	if v, ok := profile.GetAnnotations()[kubernetes.ProviderAnnotationKey]; ok {
		if ddai.Annotations == nil {
			ddai.Annotations = make(map[string]string)
		}
		ddai.Annotations[kubernetes.ProviderAnnotationKey] = v
	}
	// todo (mackjmr): Remove this once Host Profiler annotation support is removed (superseded by spec.features.hostProfiler).
	// Propagate the host-profiler experimental annotations from the profile onto the DDAI
	// so a DAP can enable/configure host profiler only on the nodes it targets, independent
	// of the DDA-level annotation.
	imageOverrideAnnotationKey := fmt.Sprintf("%s/%s", experimental.ExperimentalAnnotationPrefix, experimental.ExperimentalImageOverrideConfigSubkey)
	for _, annotationKey := range []string{featureutils.EnableHostProfilerAnnotation, featureutils.EnableHostProfilerSeccompAnnotation, featureutils.EnableHostProfilerLoggingSeccompAnnotation, featureutils.EnableHostProfilerNonRootAnnotation, featureutils.HostProfilerSELinuxTypeAnnotation, imageOverrideAnnotationKey} {
		if v, ok := profile.GetAnnotations()[annotationKey]; ok {
			if ddai.Annotations == nil {
				ddai.Annotations = make(map[string]string)
			}
			ddai.Annotations[annotationKey] = v
		}
	}
	return nil
}

// addDDAIStatusToProfileStatus surfaces the profile's DatadogAgentInternal
// reconcile-error condition on the DatadogAgentProfile itself. Unlike the DDA's
// conditions (rebuilt from scratch every reconcile), DatadogAgentProfile
// conditions persist across reconciles, so the condition must be explicitly
// set to False on recovery or a stale error would linger forever.
func (r *Reconciler) addDDAIStatusToProfileStatus(ctx context.Context, profile *v1alpha1.DatadogAgentProfile, ddaiName, ddaiNamespace string, now metav1.Time) {
	ddaiName = getProfileDDAIName(ddaiName, profile.Name, profile.Namespace)
	ddai := &v1alpha1.DatadogAgentInternal{}
	if err := r.client.Get(ctx, types.NamespacedName{Name: ddaiName, Namespace: ddaiNamespace}, ddai); err != nil {
		if !apierrors.IsNotFound(err) {
			ctrl.LoggerFrom(ctx).Error(err, "unexpected error during DDAI get", "datadogagentprofile", profile.Name, "datadogagentprofile_namespace", profile.Namespace)
			return
		}
		// Leave a new profile untouched until its DDAI is created, but clear
		// an error left behind after a previously existing DDAI is removed.
		if apimeta.FindStatusCondition(profile.Status.Conditions, agentprofile.DDAIReconcileErrorConditionType) != nil {
			newCondition := agentprofile.NewDatadogAgentProfileCondition(agentprofile.DDAIReconcileErrorConditionType, metav1.ConditionFalse, now, agentprofile.DDAIReconcileOKConditionReason, "")
			profile.Status.Conditions = agentprofile.SetDatadogAgentProfileCondition(profile.Status.Conditions, newCondition)
		}
		return
	}

	var newCondition metav1.Condition
	if ddaiErrCond := condition.GetDDAICondition(&ddai.Status, common.DatadogAgentReconcileErrorConditionType); ddaiErrCond != nil && ddaiErrCond.Status == metav1.ConditionTrue {
		newCondition = agentprofile.NewDatadogAgentProfileCondition(agentprofile.DDAIReconcileErrorConditionType, metav1.ConditionTrue, now, agentprofile.DDAIReconcileErrorConditionReason, fmt.Sprintf("%s: %s", ddai.Name, ddaiErrCond.Message))
	} else {
		newCondition = agentprofile.NewDatadogAgentProfileCondition(agentprofile.DDAIReconcileErrorConditionType, metav1.ConditionFalse, now, agentprofile.DDAIReconcileOKConditionReason, "")
	}
	profile.Status.Conditions = agentprofile.SetDatadogAgentProfileCondition(profile.Status.Conditions, newCondition)
}

// getProfileDDAIName returns the name of the DDAI when profiles are used.
// Default profiles use the DDA name. User created profiles use the profile name.
func getProfileDDAIName(ddaiName, profileName, profileNamespace string) string {
	if agentprofile.IsDefaultProfile(profileNamespace, profileName) {
		return ddaiName
	}
	return profileName
}

// The node agent component override is non-nil from the default DDAI creation
func setProfileNodeAgentOverride(ddai *v1alpha1.DatadogAgentInternal, profile *v1alpha1.DatadogAgentProfile) {
	ensureOverrideExists(ddai, v2alpha1.NodeAgentComponentName)
	setProfileDDAILabels(ddai.Spec.Override[v2alpha1.NodeAgentComponentName], profile)

	// Set the DaemonSet name override for profile DDAIs to prevent conflicts
	if !agentprofile.IsDefaultProfile(profile.Namespace, profile.Name) {
		dsName := agentprofile.DaemonSetName(types.NamespacedName{
			Name:      profile.Name,
			Namespace: profile.Namespace,
		}, true) // Use v3 metadata naming

		if dsName != "" {
			ddai.Spec.Override[v2alpha1.NodeAgentComponentName].Name = &dsName
		}
	}
}

func setProfileDDAILabels(override *v2alpha1.DatadogAgentComponentOverride, profile *v1alpha1.DatadogAgentProfile) {
	if override.Labels == nil {
		override.Labels = make(map[string]string)
	}
	override.Labels[constants.ProfileLabelKey] = profile.Name
}
