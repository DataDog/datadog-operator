// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fleet

import (
	"context"
	"errors"
	"testing"

	admissionv1 "k8s.io/api/admission/v1"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	v2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
)

type mutateFenceOnDatadogAgentDeleteClient struct {
	client.Client
	mutated bool
}

func (c *mutateFenceOnDatadogAgentDeleteClient) Delete(ctx context.Context, object client.Object, opts ...client.DeleteOption) error {
	if err := c.Client.Delete(ctx, object, opts...); err != nil {
		return err
	}
	if c.mutated {
		return nil
	}
	if _, ok := object.(*v2alpha1.DatadogAgent); !ok {
		return nil
	}
	c.mutated = true
	fence := &corev1.ConfigMap{}
	if err := c.Client.Get(ctx, uninstallFenceKey, fence); err != nil {
		return err
	}
	if fence.Annotations == nil {
		fence.Annotations = make(map[string]string)
	}
	fence.Annotations["test.datadoghq.com/drift"] = "true"
	return c.Client.Update(ctx, fence)
}

func TestUninstallDatadogAgentActivatesFence(t *testing.T) {
	const deleteConfigID = "delete-config"
	d, c, _ := testManagedAgentInstallationDaemon(testManagedAgentInstallationInstallerConfig(deleteConfigID, OperationDelete, ""), nil, testFleetOwnedDDA("create-config"))
	req := testSignedManagedAgentInstallationRequest(d, methodUninstallDatadogAgent, deleteConfigID)

	_, err := d.uninstallDatadogAgent(context.Background(), req)
	require.NoError(t, err)

	fence := &corev1.ConfigMap{}
	require.NoError(t, c.Get(context.Background(), uninstallFenceKey, fence))
	assert.Equal(t, uninstallFenceStateActive, fence.Data[uninstallFenceStateKey])
	assert.Equal(t, req.Params.InstallationID, fence.Data[uninstallFenceInstallationIDKey])
	assert.Equal(t, req.Params.OperationID, fence.Data[uninstallFenceOperationIDKey])
	assert.Equal(t, req.Params.Version, fence.Data[uninstallFenceConfigIDKey])
	assert.Equal(t, req.ID, fence.Data[uninstallFenceTaskIDKey])
	assert.NotEmpty(t, fence.Data[uninstallFenceWebhookResourceVersionKey])
}

func TestUninstallDatadogAgentRejectsFenceDrift(t *testing.T) {
	const deleteConfigID = "delete-config"
	d, c, _ := testManagedAgentInstallationDaemon(testManagedAgentInstallationInstallerConfig(deleteConfigID, OperationDelete, ""), nil, testFleetOwnedDDA("create-config"))
	mutating := &mutateFenceOnDatadogAgentDeleteClient{Client: c}
	d.client = mutating
	d.apiReader = mutating
	req := testSignedManagedAgentInstallationRequest(d, methodUninstallDatadogAgent, deleteConfigID)

	_, err := d.uninstallDatadogAgent(context.Background(), req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "fence changed")
	assert.True(t, mutating.mutated)
}

func TestVerifyDatadogAgentUninstalledRejectsWebhookDrift(t *testing.T) {
	const deleteConfigID = "delete-config"
	d, c, _ := testManagedAgentInstallationDaemon(testManagedAgentInstallationInstallerConfig(deleteConfigID, OperationDelete, ""), nil)
	uninstall := testSignedManagedAgentInstallationRequest(d, methodUninstallDatadogAgent, deleteConfigID)
	_, err := d.uninstallDatadogAgent(context.Background(), uninstall)
	require.NoError(t, err)

	configuration := &admissionregistrationv1.ValidatingWebhookConfiguration{}
	require.NoError(t, c.Get(context.Background(), client.ObjectKey{Name: uninstallFenceWebhookConfigurationName}, configuration))
	configuration.Annotations = map[string]string{"test.datadoghq.com/drift": "true"}
	require.NoError(t, c.Update(context.Background(), configuration))

	verify := testSignedManagedAgentInstallationRequest(d, methodVerifyDatadogAgentUninstalled, deleteConfigID)
	verify.ID = "verify-task"
	_, err = d.verifyDatadogAgentUninstalled(context.Background(), verify)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "webhook changed after activation")
}

func TestActiveUninstallFenceBlocksManagedAgentInstallationWrites(t *testing.T) {
	configs := testManagedAgentInstallationInstallerConfig("delete-config", OperationDelete, "")
	configs["create-config"] = testManagedAgentInstallationInstallerConfig("create-config", OperationCreate, `{"spec":{}}`)["create-config"]
	d, c, _ := testManagedAgentInstallationDaemon(configs, nil, testFleetCredentialSecret())
	uninstall := testSignedManagedAgentInstallationRequest(d, methodUninstallDatadogAgent, "delete-config")
	_, err := d.activateUninstallFence(context.Background(), uninstall)
	require.NoError(t, err)

	install := testSignedManagedAgentInstallationRequest(d, methodInstallDatadogAgent, "create-config")
	_, err = d.dispatchRemoteAPIRequest(context.Background(), install)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "active add-on uninstall fence")
	assert.Error(t, c.Get(context.Background(), testDDANSN, &v2alpha1.DatadogAgent{}))
}

func TestVerifyDatadogAgentUninstalledReadsLiveState(t *testing.T) {
	const deleteConfigID = "delete-config"
	d, c, _ := testManagedAgentInstallationDaemon(testManagedAgentInstallationInstallerConfig(deleteConfigID, OperationDelete, ""), nil)
	uninstall := testSignedManagedAgentInstallationRequest(d, methodUninstallDatadogAgent, deleteConfigID)
	_, err := d.uninstallDatadogAgent(context.Background(), uninstall)
	require.NoError(t, err)

	verify := testSignedManagedAgentInstallationRequest(d, methodVerifyDatadogAgentUninstalled, deleteConfigID)
	verify.ID = "verify-task"
	_, err = d.verifyDatadogAgentUninstalled(context.Background(), verify)
	require.NoError(t, err)

	unmanaged := &v2alpha1.DatadogAgent{ObjectMeta: metav1.ObjectMeta{Namespace: "monitoring", Name: "customer-agent"}}
	require.NoError(t, c.Create(context.Background(), unmanaged))
	_, err = d.verifyDatadogAgentUninstalled(context.Background(), verify)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unmanaged DatadogAgent")
	assert.NoError(t, c.Get(context.Background(), client.ObjectKeyFromObject(unmanaged), &v2alpha1.DatadogAgent{}))
}

func TestClearDatadogAgentUninstallFence(t *testing.T) {
	const deleteConfigID = "delete-config"
	d, c, _ := testManagedAgentInstallationDaemon(testManagedAgentInstallationInstallerConfig(deleteConfigID, OperationDelete, ""), nil)
	uninstall := testSignedManagedAgentInstallationRequest(d, methodUninstallDatadogAgent, deleteConfigID)
	_, err := d.activateUninstallFence(context.Background(), uninstall)
	require.NoError(t, err)

	clear := testSignedManagedAgentInstallationRequest(d, methodClearDatadogAgentUninstallFence, deleteConfigID)
	clear.ID = "clear-task"
	_, err = d.clearDatadogAgentUninstallFence(context.Background(), clear)
	require.NoError(t, err)

	fence := &corev1.ConfigMap{}
	require.NoError(t, c.Get(context.Background(), uninstallFenceKey, fence))
	assert.Equal(t, uninstallFenceStateInactive, fence.Data[uninstallFenceStateKey])
	assert.Empty(t, fence.Data[uninstallFenceOperationIDKey])
}

func TestRehydrateInstallerStateValidatesActiveFence(t *testing.T) {
	const deleteConfigID = "delete-config"
	configs := testManagedAgentInstallationInstallerConfig(deleteConfigID, OperationDelete, "")
	d, c, rc := testManagedAgentInstallationDaemon(configs, []*pbgo.PackageState{{
		Package:             packageDatadogOperator,
		StableConfigVersion: "unknown",
	}})
	req := testSignedManagedAgentInstallationRequest(d, methodUninstallDatadogAgent, deleteConfigID)
	_, err := d.activateUninstallFence(context.Background(), req)
	require.NoError(t, err)
	configuration := &admissionregistrationv1.ValidatingWebhookConfiguration{}
	require.NoError(t, c.Get(context.Background(), client.ObjectKey{Name: uninstallFenceWebhookConfigurationName}, configuration))
	configuration.Annotations = map[string]string{"test.datadoghq.com/rotation": "true"}
	require.NoError(t, c.Update(context.Background(), configuration))

	require.NoError(t, d.rehydrateInstallerState(context.Background()))
	assert.Empty(t, rc.state[0].StableConfigVersion)

	fence := &corev1.ConfigMap{}
	require.NoError(t, c.Get(context.Background(), uninstallFenceKey, fence))
	require.NoError(t, c.Get(context.Background(), client.ObjectKey{Name: uninstallFenceWebhookConfigurationName}, configuration))
	assert.Equal(t, configuration.ResourceVersion, fence.Data[uninstallFenceWebhookResourceVersionKey])
	fence.Data[uninstallFenceInstallationIDKey] = "223e4567-e89b-42d3-a456-426614174000"
	require.NoError(t, c.Update(context.Background(), fence))
	err = d.rehydrateInstallerState(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "different managed installation")
}

func TestRehydrateInstallerStateMarksDatadogAgentBehindActiveFencePartial(t *testing.T) {
	const deleteConfigID = "delete-config"
	d, c, rc := testManagedAgentInstallationDaemon(
		testManagedAgentInstallationInstallerConfig(deleteConfigID, OperationDelete, ""),
		[]*pbgo.PackageState{{Package: packageDatadogOperator, StableConfigVersion: "create-config"}},
		testFleetOwnedDDA("create-config"),
	)
	req := testSignedManagedAgentInstallationRequest(d, methodUninstallDatadogAgent, deleteConfigID)
	_, err := d.activateUninstallFence(context.Background(), req)
	require.NoError(t, err)

	require.NoError(t, d.rehydrateInstallerState(context.Background()))
	assert.Equal(t, fleetPartialConfigVersionPrefix+"create-config", rc.state[0].StableConfigVersion)
	req.ExpectedState.StableConfig = fleetPartialConfigVersionPrefix + "create-config"
	require.NoError(t, d.handleTask(context.Background(), req))
	assert.Empty(t, rc.state[0].StableConfigVersion)
	assert.Error(t, c.Get(context.Background(), testDDANSN, &v2alpha1.DatadogAgent{}))
}

func TestManagedAgentInstallationAdmissionHandler(t *testing.T) {
	request := admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
		Operation: admissionv1.Create,
		Namespace: fleetDatadogAgentNamespace,
		Name:      fleetDatadogAgentName,
		Resource: metav1.GroupVersionResource{
			Group:    v2alpha1.GroupVersion.Group,
			Version:  v2alpha1.GroupVersion.Version,
			Resource: "datadogagents",
		},
	}}

	d, c, _ := testManagedAgentInstallationDaemon(nil, nil)
	handler := &uninstallFenceAdmissionHandler{reader: c, identity: testManagedAgentInstallationIdentity}
	assert.True(t, handler.Handle(context.Background(), request).Allowed)

	d.configs = testManagedAgentInstallationInstallerConfig("delete-config", OperationDelete, "")
	uninstall := testSignedManagedAgentInstallationRequest(d, methodUninstallDatadogAgent, "delete-config")
	_, err := d.activateUninstallFence(context.Background(), uninstall)
	require.NoError(t, err)
	response := handler.Handle(context.Background(), request)
	assert.False(t, response.Allowed)
	assert.Contains(t, response.Result.Message, uninstallFenceDenialMessage)
	require.Len(t, response.Result.Details.Causes, 1)
	assert.Equal(t, uninstallFenceDenialCauseField, response.Result.Details.Causes[0].Field)
	assert.True(t, isUninstallFenceDenial(&apierrors.StatusError{ErrStatus: *response.Result}))
	assert.False(t, isUninstallFenceDenial(errors.New(uninstallFenceDenialMessage)))

	request.Namespace = "monitoring"
	request.Name = "customer-agent"
	assert.True(t, handler.Handle(context.Background(), request).Allowed)
	request.Namespace = managedAgentInstallationWindowsProfileKey.Namespace
	request.Name = managedAgentInstallationWindowsProfileKey.Name
	request.Resource = metav1.GroupVersionResource{
		Group:    v1alpha1.GroupVersion.Group,
		Version:  v1alpha1.GroupVersion.Version,
		Resource: "datadogagentprofiles",
	}
	assert.False(t, handler.Handle(context.Background(), request).Allowed)
	request.Name = "customer-profile"
	assert.True(t, handler.Handle(context.Background(), request).Allowed)

	request.Name = managedAgentInstallationWindowsProfileKey.Name
	request.Operation = admissionv1.Delete
	assert.True(t, handler.Handle(context.Background(), request).Allowed)
}

func TestValidateUninstallFenceWebhook(t *testing.T) {
	failurePolicy := admissionregistrationv1.Fail
	path := uninstallFenceAdmissionPath
	sideEffects := admissionregistrationv1.SideEffectClassNone
	webhook := &admissionregistrationv1.ValidatingWebhook{
		Name:          uninstallFenceWebhookName,
		FailurePolicy: &failurePolicy,
		SideEffects:   &sideEffects,
		ClientConfig: admissionregistrationv1.WebhookClientConfig{
			CABundle: []byte("ca"),
			Service: &admissionregistrationv1.ServiceReference{
				Namespace: uninstallFenceWebhookDefaultNamespace,
				Name:      uninstallFenceWebhookServiceName,
				Path:      &path,
			},
		},
		Rules: []admissionregistrationv1.RuleWithOperations{
			{
				Operations: []admissionregistrationv1.OperationType{admissionregistrationv1.Create, admissionregistrationv1.Update},
				Rule: admissionregistrationv1.Rule{
					APIGroups:   []string{v2alpha1.GroupVersion.Group},
					APIVersions: []string{v2alpha1.GroupVersion.Version},
					Resources:   []string{"datadogagents"},
				},
			},
			{
				Operations: []admissionregistrationv1.OperationType{admissionregistrationv1.Create, admissionregistrationv1.Update},
				Rule: admissionregistrationv1.Rule{
					APIGroups:   []string{v1alpha1.GroupVersion.Group},
					APIVersions: []string{v1alpha1.GroupVersion.Version},
					Resources:   []string{"datadogagentprofiles"},
				},
			},
		},
	}

	require.NoError(t, validateUninstallFenceWebhook(webhook, uninstallFenceWebhookDefaultNamespace))
	webhook.FailurePolicy = nil
	require.Error(t, validateUninstallFenceWebhook(webhook, uninstallFenceWebhookDefaultNamespace))
}

func TestSetUninstallFenceWebhookMode(t *testing.T) {
	failurePolicy := admissionregistrationv1.Ignore
	configuration := &admissionregistrationv1.ValidatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{Name: uninstallFenceWebhookConfigurationName},
		Webhooks: []admissionregistrationv1.ValidatingWebhook{{
			Name:          uninstallFenceWebhookName,
			FailurePolicy: &failurePolicy,
		}},
	}
	c := fake.NewClientBuilder().WithScheme(testFleetScheme()).WithObjects(configuration).Build()
	d := &Daemon{client: c, apiReader: c}

	require.NoError(t, d.setUninstallFenceWebhookMode(context.Background(), true))
	require.NoError(t, c.Get(context.Background(), client.ObjectKeyFromObject(configuration), configuration))
	require.NotNil(t, configuration.Webhooks[0].FailurePolicy)
	assert.Equal(t, admissionregistrationv1.Fail, *configuration.Webhooks[0].FailurePolicy)

	require.NoError(t, d.setUninstallFenceWebhookMode(context.Background(), false))
	require.NoError(t, c.Get(context.Background(), client.ObjectKeyFromObject(configuration), configuration))
	require.NotNil(t, configuration.Webhooks[0].FailurePolicy)
	assert.Equal(t, admissionregistrationv1.Ignore, *configuration.Webhooks[0].FailurePolicy)
}
