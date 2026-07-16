// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fleet

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
	"os"
	"slices"

	admissionv1 "k8s.io/api/admission/v1"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	k8sretry "k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	v1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	v2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/pkg/remoteconfig"
)

const (
	uninstallFenceConfigMapName             = "datadog-agent-uninstall-fence"
	uninstallFenceWebhookConfigurationName  = "datadog-agent-uninstall-fence"
	uninstallFenceWebhookName               = "datadog-agent-uninstall-fence.datadoghq.com"
	uninstallFenceWebhookServiceName        = "datadog-operator-webhook-service"
	uninstallFenceWebhookDefaultNamespace   = "datadog"
	uninstallFenceAdmissionPath             = "/validate-datadoghq-com-v2alpha1-datadogagent-uninstall-fence"
	uninstallFenceDenialMessage             = "DatadogAgent writes are blocked while the Datadog Operator add-on is being removed"
	uninstallFenceDenialCauseField          = "datadoghq.com/uninstall-fence"
	uninstallFenceStateKey                  = "state"
	uninstallFenceStateActive               = "active"
	uninstallFenceStateInactive             = "inactive"
	uninstallFenceInstallationIDKey         = "installation_id"
	uninstallFenceOperationIDKey            = "operation_id"
	uninstallFenceConfigIDKey               = "config_id"
	uninstallFenceTaskIDKey                 = "task_id"
	uninstallFenceWebhookResourceVersionKey = "webhook_resource_version"
)

func uninstallFenceWebhookServiceNamespace() string {
	if namespace := os.Getenv("POD_NAMESPACE"); namespace != "" {
		return namespace
	}
	return uninstallFenceWebhookDefaultNamespace
}

var uninstallFenceKey = types.NamespacedName{
	Namespace: fleetDatadogAgentNamespace,
	Name:      uninstallFenceConfigMapName,
}

type uninstallFenceAdmissionHandler struct {
	reader   client.Reader
	identity remoteconfig.ManagedAgentInstallationIdentity
}

func RegisterUninstallFenceWebhook(mgr manager.Manager, identity remoteconfig.ManagedAgentInstallationIdentity) {
	mgr.GetWebhookServer().Register(uninstallFenceAdmissionPath, &admission.Webhook{
		Handler: &uninstallFenceAdmissionHandler{reader: mgr.GetAPIReader(), identity: identity},
	})
}

func (h *uninstallFenceAdmissionHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	if req.Operation != admissionv1.Create && req.Operation != admissionv1.Update {
		return admission.Allowed("")
	}
	isDatadogAgent := req.Resource.Group == v2alpha1.GroupVersion.Group &&
		req.Resource.Version == v2alpha1.GroupVersion.Version &&
		req.Resource.Resource == "datadogagents"
	isDatadogAgentProfile := req.Resource.Group == v1alpha1.GroupVersion.Group &&
		req.Resource.Version == v1alpha1.GroupVersion.Version &&
		req.Resource.Resource == "datadogagentprofiles"
	if !isDatadogAgent && !isDatadogAgentProfile {
		return admission.Allowed("")
	}
	if isDatadogAgent && (req.Namespace != fleetDatadogAgentNamespace || req.Name != fleetDatadogAgentName) {
		return admission.Allowed("")
	}
	if isDatadogAgentProfile && (req.Namespace != managedAgentInstallationWindowsProfileKey.Namespace || req.Name != managedAgentInstallationWindowsProfileKey.Name) {
		return admission.Allowed("")
	}

	fence := &corev1.ConfigMap{}
	if err := h.reader.Get(ctx, uninstallFenceKey, fence); err != nil {
		return admission.Errored(http.StatusInternalServerError, fmt.Errorf("read DatadogAgent uninstall fence: %w", err))
	}
	if fence.Data[uninstallFenceStateKey] == uninstallFenceStateInactive {
		return admission.Allowed("")
	}
	if err := validateActiveUninstallFenceIdentity(fence, h.identity); err != nil {
		return denyForUninstallFence(uninstallFenceDenialMessage + ": fence state is invalid")
	}
	return denyForUninstallFence(uninstallFenceDenialMessage)
}

func denyForUninstallFence(message string) admission.Response {
	response := admission.Denied(message)
	response.Result.Details = &metav1.StatusDetails{
		Causes: []metav1.StatusCause{{
			Type:    metav1.CauseTypeForbidden,
			Message: uninstallFenceDenialMessage,
			Field:   uninstallFenceDenialCauseField,
		}},
	}
	return response
}

func isUninstallFenceDenial(err error) bool {
	var statusErr *apierrors.StatusError
	if !errors.As(err, &statusErr) || statusErr.ErrStatus.Details == nil {
		return false
	}
	for _, cause := range statusErr.ErrStatus.Details.Causes {
		if cause.Type == metav1.CauseTypeForbidden && cause.Field == uninstallFenceDenialCauseField {
			return true
		}
	}
	return false
}

func (d *Daemon) activateUninstallFence(ctx context.Context, req remoteAPIRequest) (*corev1.ConfigMap, error) {
	err := k8sretry.RetryOnConflict(k8sretry.DefaultBackoff, func() error {
		current := &corev1.ConfigMap{}
		if err := d.managedAgentInstallationReader().Get(ctx, uninstallFenceKey, current); err != nil {
			return fmt.Errorf("read uninstall fence ConfigMap %s/%s: %w", uninstallFenceKey.Namespace, uninstallFenceKey.Name, err)
		}
		switch current.Data[uninstallFenceStateKey] {
		case uninstallFenceStateActive:
			if err := validateActiveUninstallFenceRequest(current, d.managedAgentInstallationIdentity, req); err != nil {
				return err
			}
			return nil
		case "", uninstallFenceStateInactive:
		default:
			return &stateDoesntMatchError{msg: fmt.Sprintf("uninstall fence ConfigMap %s/%s has unknown state %q", current.Namespace, current.Name, current.Data[uninstallFenceStateKey])}
		}

		base := current.DeepCopy()
		if current.Data == nil {
			current.Data = make(map[string]string)
		}
		current.Data[uninstallFenceStateKey] = uninstallFenceStateActive
		current.Data[uninstallFenceInstallationIDKey] = req.Params.InstallationID
		current.Data[uninstallFenceOperationIDKey] = req.Params.OperationID
		current.Data[uninstallFenceConfigIDKey] = req.Params.Version
		current.Data[uninstallFenceTaskIDKey] = req.ID
		delete(current.Data, uninstallFenceWebhookResourceVersionKey)
		if err := d.client.Patch(ctx, current, client.MergeFromWithOptions(base, client.MergeFromWithOptimisticLock{}), client.FieldOwner("fleet-daemon")); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if err := d.setUninstallFenceWebhookMode(ctx, true); err != nil {
		return nil, fmt.Errorf("enable uninstall fence webhook mode: %w", err)
	}
	if _, err := d.pinUninstallFenceWebhookRevision(ctx, &req, false); err != nil {
		return nil, fmt.Errorf("pin uninstall fence webhook revision: %w", err)
	}
	return d.verifyActiveUninstallFence(ctx, req)
}

func (d *Daemon) verifyActiveUninstallFence(ctx context.Context, req remoteAPIRequest) (*corev1.ConfigMap, error) {
	fence := &corev1.ConfigMap{}
	if err := d.managedAgentInstallationReader().Get(ctx, uninstallFenceKey, fence); err != nil {
		return nil, fmt.Errorf("read active uninstall fence ConfigMap %s/%s: %w", uninstallFenceKey.Namespace, uninstallFenceKey.Name, err)
	}
	if err := validateActiveUninstallFenceRequest(fence, d.managedAgentInstallationIdentity, req); err != nil {
		return nil, err
	}
	if err := d.verifyUninstallFence(ctx, fence); err != nil {
		return nil, err
	}
	return fence, nil
}

func (d *Daemon) verifyUnchangedActiveUninstallFence(ctx context.Context, req remoteAPIRequest, expected *corev1.ConfigMap) error {
	current, err := d.verifyActiveUninstallFence(ctx, req)
	if err != nil {
		return err
	}
	if expected == nil || expected.ResourceVersion == "" || current.ResourceVersion != expected.ResourceVersion {
		return &stateDoesntMatchError{msg: "DatadogAgent uninstall fence changed during managed Agent installation verification"}
	}
	return nil
}

func (d *Daemon) verifyUninstallFence(ctx context.Context, fence *corev1.ConfigMap) error {
	configuration, err := d.readUninstallFenceWebhookConfiguration(ctx)
	if err != nil {
		return err
	}
	if fence.Data[uninstallFenceWebhookResourceVersionKey] == "" ||
		fence.Data[uninstallFenceWebhookResourceVersionKey] != configuration.ResourceVersion {
		return &stateDoesntMatchError{msg: "uninstall fence webhook changed after activation"}
	}
	if d.fenceVerifier != nil {
		return d.fenceVerifier(ctx, fence)
	}

	probeName := fmt.Sprintf("fleet-fence-probe-%x", sha256.Sum256([]byte(fence.ResourceVersion+fence.Data[uninstallFenceOperationIDKey])))[:30]
	probe := &v2alpha1.DatadogAgent{ObjectMeta: metav1.ObjectMeta{Namespace: fleetDatadogAgentNamespace, Name: probeName}}
	err = d.client.Create(ctx, probe, client.DryRunAll)
	if err == nil {
		return fmt.Errorf("uninstall fence dry-run DatadogAgent create was not denied")
	}
	if !isUninstallFenceDenial(err) {
		return fmt.Errorf("uninstall fence dry-run DatadogAgent create returned an unexpected error: %w", err)
	}
	return nil
}

func (d *Daemon) readUninstallFenceWebhookConfiguration(ctx context.Context) (*admissionregistrationv1.ValidatingWebhookConfiguration, error) {
	configuration := &admissionregistrationv1.ValidatingWebhookConfiguration{}
	if err := d.managedAgentInstallationReader().Get(ctx, types.NamespacedName{Name: uninstallFenceWebhookConfigurationName}, configuration); err != nil {
		return nil, fmt.Errorf("read uninstall fence ValidatingWebhookConfiguration: %w", err)
	}
	for i := range configuration.Webhooks {
		webhook := &configuration.Webhooks[i]
		if webhook.Name != uninstallFenceWebhookName {
			continue
		}
		if err := validateUninstallFenceWebhook(webhook, uninstallFenceWebhookServiceNamespace()); err != nil {
			return nil, err
		}
		return configuration, nil
	}
	return nil, fmt.Errorf("uninstall fence webhook %q is missing", uninstallFenceWebhookName)
}

func (d *Daemon) pinUninstallFenceWebhookRevision(ctx context.Context, req *remoteAPIRequest, allowRevisionUpdate bool) (*corev1.ConfigMap, error) {
	configuration, readErr := d.readUninstallFenceWebhookConfiguration(ctx)
	if readErr != nil {
		return nil, readErr
	}
	var pinned *corev1.ConfigMap
	retryErr := k8sretry.RetryOnConflict(k8sretry.DefaultBackoff, func() error {
		fence := &corev1.ConfigMap{}
		if err := d.managedAgentInstallationReader().Get(ctx, uninstallFenceKey, fence); err != nil {
			return err
		}
		if err := validateActiveUninstallFenceIdentity(fence, d.managedAgentInstallationIdentity); err != nil {
			return err
		}
		if req != nil {
			if err := validateActiveUninstallFenceRequest(fence, d.managedAgentInstallationIdentity, *req); err != nil {
				return err
			}
		}
		currentRevision := fence.Data[uninstallFenceWebhookResourceVersionKey]
		if currentRevision != "" {
			if currentRevision != configuration.ResourceVersion {
				if !allowRevisionUpdate {
					return &stateDoesntMatchError{msg: "uninstall fence webhook changed after activation"}
				}
			} else {
				pinned = fence
				return nil
			}
		}
		base := fence.DeepCopy()
		fence.Data[uninstallFenceWebhookResourceVersionKey] = configuration.ResourceVersion
		if err := d.client.Patch(ctx, fence, client.MergeFromWithOptions(base, client.MergeFromWithOptimisticLock{}), client.FieldOwner("fleet-daemon")); err != nil {
			return err
		}
		pinned = fence
		return nil
	})
	if retryErr != nil {
		return nil, retryErr
	}
	return pinned, nil
}

func (d *Daemon) setUninstallFenceWebhookMode(ctx context.Context, active bool) error {
	if d.fenceModeManager != nil {
		return d.fenceModeManager(ctx, active)
	}
	wantPolicy := admissionregistrationv1.Ignore
	if active {
		wantPolicy = admissionregistrationv1.Fail
	}
	if err := k8sretry.RetryOnConflict(k8sretry.DefaultBackoff, func() error {
		configuration := &admissionregistrationv1.ValidatingWebhookConfiguration{}
		if err := d.managedAgentInstallationReader().Get(ctx, types.NamespacedName{Name: uninstallFenceWebhookConfigurationName}, configuration); err != nil {
			return err
		}
		for i := range configuration.Webhooks {
			webhook := &configuration.Webhooks[i]
			if webhook.Name != uninstallFenceWebhookName {
				continue
			}
			if webhook.FailurePolicy != nil && *webhook.FailurePolicy == wantPolicy {
				return nil
			}
			base := configuration.DeepCopy()
			webhook.FailurePolicy = &wantPolicy
			return d.client.Patch(ctx, configuration, client.MergeFromWithOptions(base, client.MergeFromWithOptimisticLock{}), client.FieldOwner("fleet-daemon"))
		}
		return fmt.Errorf("uninstall fence webhook %q is missing", uninstallFenceWebhookName)
	}); err != nil {
		return err
	}
	return d.validateUninstallFenceWebhookMode(ctx, wantPolicy)
}

func (d *Daemon) validateUninstallFenceWebhookMode(ctx context.Context, policy admissionregistrationv1.FailurePolicyType) error {
	configuration := &admissionregistrationv1.ValidatingWebhookConfiguration{}
	if err := d.managedAgentInstallationReader().Get(ctx, types.NamespacedName{Name: uninstallFenceWebhookConfigurationName}, configuration); err != nil {
		return err
	}
	for i := range configuration.Webhooks {
		webhook := &configuration.Webhooks[i]
		if webhook.Name != uninstallFenceWebhookName {
			continue
		}
		if webhook.FailurePolicy == nil || *webhook.FailurePolicy != policy {
			return fmt.Errorf("uninstall fence webhook failurePolicy must be %s", policy)
		}
		return nil
	}
	return fmt.Errorf("uninstall fence webhook %q is missing", uninstallFenceWebhookName)
}

func validateUninstallFenceWebhook(webhook *admissionregistrationv1.ValidatingWebhook, serviceNamespace string) error {
	if webhook.FailurePolicy == nil || *webhook.FailurePolicy != admissionregistrationv1.Fail {
		return fmt.Errorf("uninstall fence webhook failurePolicy must be Fail")
	}
	if webhook.ClientConfig.Service == nil ||
		webhook.ClientConfig.Service.Name != uninstallFenceWebhookServiceName ||
		webhook.ClientConfig.Service.Namespace != serviceNamespace ||
		webhook.ClientConfig.Service.Path == nil || *webhook.ClientConfig.Service.Path != uninstallFenceAdmissionPath {
		return fmt.Errorf("uninstall fence webhook service target is invalid")
	}
	if len(webhook.ClientConfig.CABundle) == 0 {
		return fmt.Errorf("uninstall fence webhook CA bundle is empty")
	}
	if webhook.NamespaceSelector != nil || webhook.ObjectSelector != nil || len(webhook.MatchConditions) != 0 {
		return fmt.Errorf("uninstall fence webhook must not use selectors or match conditions")
	}
	if webhook.SideEffects == nil || (*webhook.SideEffects != admissionregistrationv1.SideEffectClassNone && *webhook.SideEffects != admissionregistrationv1.SideEffectClassNoneOnDryRun) {
		return fmt.Errorf("uninstall fence webhook must support dry-run requests")
	}

	if len(webhook.Rules) != 2 {
		return fmt.Errorf("uninstall fence webhook must have exactly two rules")
	}
	if err := validateUninstallFenceRule(webhook.Rules[0], v2alpha1.GroupVersion.Group, v2alpha1.GroupVersion.Version, "datadogagents"); err != nil {
		return fmt.Errorf("uninstall fence DatadogAgent rule: %w", err)
	}
	if err := validateUninstallFenceRule(webhook.Rules[1], v1alpha1.GroupVersion.Group, v1alpha1.GroupVersion.Version, "datadogagentprofiles"); err != nil {
		return fmt.Errorf("uninstall fence DatadogAgentProfile rule: %w", err)
	}
	return nil
}

func validateUninstallFenceRule(rule admissionregistrationv1.RuleWithOperations, group, version, resource string) error {
	if len(rule.Rule.APIGroups) != 1 || rule.Rule.APIGroups[0] != group ||
		len(rule.Rule.APIVersions) != 1 || rule.Rule.APIVersions[0] != version ||
		len(rule.Rule.Resources) != 1 || rule.Rule.Resources[0] != resource {
		return fmt.Errorf("resource selector is invalid")
	}
	if len(rule.Operations) != 2 ||
		!containsOperation(rule.Operations, admissionregistrationv1.Create) ||
		!containsOperation(rule.Operations, admissionregistrationv1.Update) {
		return fmt.Errorf("CREATE and UPDATE operations are required")
	}
	return nil
}

func (d *Daemon) ensureUninstallFenceInactive(ctx context.Context) error {
	if !d.managedAgentInstallationIdentity.Configured() {
		return nil
	}
	fence := &corev1.ConfigMap{}
	if err := d.managedAgentInstallationReader().Get(ctx, uninstallFenceKey, fence); err != nil {
		return fmt.Errorf("read DatadogAgent uninstall fence: %w", err)
	}
	switch fence.Data[uninstallFenceStateKey] {
	case "", uninstallFenceStateInactive:
		return nil
	case uninstallFenceStateActive:
		return &stateDoesntMatchError{msg: "DatadogAgent writes are blocked by an active add-on uninstall fence"}
	default:
		return &stateDoesntMatchError{msg: fmt.Sprintf("DatadogAgent uninstall fence has unknown state %q", fence.Data[uninstallFenceStateKey])}
	}
}

func (d *Daemon) rehydrateUninstallFence(ctx context.Context) (*corev1.ConfigMap, error) {
	if !d.managedAgentInstallationIdentity.Configured() {
		return nil, nil
	}
	fence := &corev1.ConfigMap{}
	if err := d.managedAgentInstallationReader().Get(ctx, uninstallFenceKey, fence); err != nil {
		return nil, fmt.Errorf("read DatadogAgent uninstall fence during startup recovery: %w", err)
	}
	switch fence.Data[uninstallFenceStateKey] {
	case "", uninstallFenceStateInactive:
		return nil, nil
	case uninstallFenceStateActive:
		if err := validateActiveUninstallFenceIdentity(fence, d.managedAgentInstallationIdentity); err != nil {
			return nil, err
		}
		if err := d.setUninstallFenceWebhookMode(ctx, true); err != nil {
			return nil, err
		}
		var err error
		fence, err = d.pinUninstallFenceWebhookRevision(ctx, nil, true)
		if err != nil {
			return nil, err
		}
		if err := d.verifyUninstallFence(ctx, fence); err != nil {
			return nil, err
		}
		return fence, nil
	default:
		return nil, &stateDoesntMatchError{msg: fmt.Sprintf("DatadogAgent uninstall fence has unknown state %q", fence.Data[uninstallFenceStateKey])}
	}
}

func (d *Daemon) revalidateRecoveredUninstallFence(ctx context.Context, recovered *corev1.ConfigMap) error {
	if recovered == nil {
		return nil
	}
	current := &corev1.ConfigMap{}
	if err := d.managedAgentInstallationReader().Get(ctx, uninstallFenceKey, current); err != nil {
		return err
	}
	if current.ResourceVersion != recovered.ResourceVersion {
		return &stateDoesntMatchError{msg: "DatadogAgent uninstall fence changed during startup recovery"}
	}
	if err := validateActiveUninstallFenceIdentity(current, d.managedAgentInstallationIdentity); err != nil {
		return err
	}
	return d.verifyUninstallFence(ctx, current)
}

func (d *Daemon) verifyDatadogAgentUninstalled(ctx context.Context, req remoteAPIRequest) (*pendingOperation, error) {
	op, err := d.resolveManagedAgentInstallationOperation(req, OperationDelete)
	if err != nil {
		return nil, err
	}
	fence, err := d.verifyActiveUninstallFence(ctx, req)
	if err != nil {
		return nil, err
	}
	if err := d.verifyDatadogAgentManagedAgentInstallationResourcesAbsent(ctx, op.NamespacedName); err != nil {
		return nil, err
	}
	if err := d.verifyUnchangedActiveUninstallFence(ctx, req, fence); err != nil {
		return nil, err
	}
	return nil, nil
}

func (d *Daemon) verifyDatadogAgentManagedAgentInstallationResourcesAbsent(ctx context.Context, target types.NamespacedName) error {
	if err := d.validateFleetDatadogAgentTargetAbsent(ctx, target); err != nil {
		return err
	}
	if err := d.verifyManagedAgentInstallationWindowsProfileAbsent(ctx); err != nil {
		return err
	}
	ddais := &v1alpha1.DatadogAgentInternalList{}
	if err := d.managedAgentInstallationReader().List(ctx, ddais, client.MatchingLabels{fleetManagedByLabel: fleetManagedByValue}); err != nil {
		return fmt.Errorf("list Fleet-managed DatadogAgentInternals: %w", err)
	}
	if len(ddais.Items) != 0 {
		return &stateDoesntMatchError{msg: fmt.Sprintf("Fleet-managed DatadogAgentInternal %s/%s remains after uninstall", ddais.Items[0].Namespace, ddais.Items[0].Name)}
	}
	removed, err := d.datadogAgentInternalClusterResourcesRemoved(ctx, managedAgentInstallationPartOfLabelValue(target))
	if err != nil {
		return fmt.Errorf("list managed DatadogAgent dependents: %w", err)
	}
	if !removed {
		return &stateDoesntMatchError{msg: "managed DatadogAgent dependents remain after uninstall"}
	}
	return nil
}

func (d *Daemon) clearDatadogAgentUninstallFence(ctx context.Context, req remoteAPIRequest) (*pendingOperation, error) {
	_, resolveErr := d.resolveManagedAgentInstallationOperation(req, OperationDelete)
	if resolveErr != nil {
		return nil, resolveErr
	}
	fence := &corev1.ConfigMap{}
	if err := d.managedAgentInstallationReader().Get(ctx, uninstallFenceKey, fence); err != nil {
		return nil, err
	}
	if err := validateActiveUninstallFenceRequest(fence, d.managedAgentInstallationIdentity, req); err != nil {
		return nil, err
	}
	if err := d.setUninstallFenceWebhookMode(ctx, true); err != nil {
		return nil, err
	}
	if _, err := d.readUninstallFenceWebhookConfiguration(ctx); err != nil {
		return nil, err
	}

	retryErr := k8sretry.RetryOnConflict(k8sretry.DefaultBackoff, func() error {
		fence := &corev1.ConfigMap{}
		if err := d.managedAgentInstallationReader().Get(ctx, uninstallFenceKey, fence); err != nil {
			return err
		}
		if err := validateActiveUninstallFenceRequest(fence, d.managedAgentInstallationIdentity, req); err != nil {
			return err
		}
		base := fence.DeepCopy()
		fence.Data[uninstallFenceStateKey] = uninstallFenceStateInactive
		for _, key := range []string{
			uninstallFenceInstallationIDKey,
			uninstallFenceOperationIDKey,
			uninstallFenceConfigIDKey,
			uninstallFenceTaskIDKey,
			uninstallFenceWebhookResourceVersionKey,
		} {
			delete(fence.Data, key)
		}
		return d.client.Patch(ctx, fence, client.MergeFromWithOptions(base, client.MergeFromWithOptimisticLock{}), client.FieldOwner("fleet-daemon"))
	})
	if retryErr != nil {
		return nil, retryErr
	}
	if err := d.setUninstallFenceWebhookMode(ctx, false); err != nil {
		return nil, fmt.Errorf("disable uninstall fence webhook mode: %w", err)
	}
	if err := d.ensureUninstallFenceInactive(ctx); err != nil {
		return nil, err
	}
	return nil, nil
}

func validateActiveUninstallFenceRequest(fence *corev1.ConfigMap, identity remoteconfig.ManagedAgentInstallationIdentity, req remoteAPIRequest) error {
	if err := validateActiveUninstallFenceIdentity(fence, identity); err != nil {
		return err
	}
	if fence.Data[uninstallFenceOperationIDKey] != req.Params.OperationID ||
		fence.Data[uninstallFenceConfigIDKey] != req.Params.Version {
		return &stateDoesntMatchError{msg: "active DatadogAgent uninstall fence belongs to a different managed Agent installation operation"}
	}
	return nil
}

func validateActiveUninstallFenceIdentity(fence *corev1.ConfigMap, identity remoteconfig.ManagedAgentInstallationIdentity) error {
	if fence.Data[uninstallFenceStateKey] != uninstallFenceStateActive {
		return &stateDoesntMatchError{msg: "DatadogAgent uninstall fence is not active"}
	}
	if !identity.Configured() || identity.Validate() != nil ||
		fence.Data[uninstallFenceInstallationIDKey] != identity.InstallationID {
		return &stateDoesntMatchError{msg: "active DatadogAgent uninstall fence belongs to a different managed installation"}
	}
	if fence.Data[uninstallFenceOperationIDKey] == "" || fence.Data[uninstallFenceConfigIDKey] == "" || fence.Data[uninstallFenceTaskIDKey] == "" {
		return &stateDoesntMatchError{msg: "active DatadogAgent uninstall fence is missing operation identity"}
	}
	return nil
}

func containsOperation(values []admissionregistrationv1.OperationType, expected admissionregistrationv1.OperationType) bool {
	return slices.Contains(values, expected)
}
