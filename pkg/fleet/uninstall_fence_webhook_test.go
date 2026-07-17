// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fleet

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type persistThenAlreadyExistsClient struct {
	client.Client
	key      types.NamespacedName
	resource string
	returned bool
}

func (c *persistThenAlreadyExistsClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	if !c.returned && client.ObjectKeyFromObject(obj) == c.key {
		c.returned = true
		if err := c.Client.Create(ctx, obj, opts...); err != nil {
			return err
		}
		return apierrors.NewAlreadyExists(schema.GroupResource{Resource: c.resource}, obj.GetName())
	}
	return c.Client.Create(ctx, obj, opts...)
}

func TestPrepareUninstallFenceWebhook(t *testing.T) {
	const podNamespace = "datadog-agent"
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, rbacv1.AddToScheme(scheme))
	require.NoError(t, admissionregistrationv1.AddToScheme(scheme))

	runtimeAnchor := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{
		Namespace: podNamespace,
		Name:      managedAgentInstallationRuntimeAnchorName,
		UID:       types.UID("runtime-anchor-uid"),
	}}
	clusterAnchor := &rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{
		Name: managedAgentInstallationRuntimeAnchorName,
		UID:  types.UID("cluster-anchor-uid"),
	}}
	intentAnchor := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{
		Namespace: managedAgentInstallationIntentKey.Namespace,
		Name:      managedAgentInstallationIntentKey.Name,
		UID:       types.UID("intent-anchor-uid"),
	}}
	kubeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(runtimeAnchor, clusterAnchor, intentAnchor).Build()

	oldCertDir := UninstallFenceAdmissionCertDir
	UninstallFenceAdmissionCertDir = t.TempDir()
	t.Cleanup(func() { UninstallFenceAdmissionCertDir = oldCertDir })

	require.NoError(t, PrepareUninstallFenceWebhook(context.Background(), kubeClient, podNamespace))

	secret := &corev1.Secret{}
	require.NoError(t, kubeClient.Get(context.Background(), types.NamespacedName{Namespace: podNamespace, Name: uninstallFenceWebhookTLSSecretName}, secret))
	require.NoError(t, validateUninstallFenceWebhookCertificate(uninstallFenceWebhookCertificateFromSecret(secret), podNamespace, metav1.Now().Time))
	serverCertificate, err := os.ReadFile(filepath.Join(UninstallFenceAdmissionCertDir, uninstallFenceWebhookCertificateName))
	require.NoError(t, err)
	require.Equal(t, secret.Data[uninstallFenceWebhookCertificateName], serverCertificate)

	fence := &corev1.ConfigMap{}
	require.NoError(t, kubeClient.Get(context.Background(), uninstallFenceKey, fence))
	require.Equal(t, uninstallFenceStateInactive, fence.Data[uninstallFenceStateKey])
	require.NoError(t, requireManagedAgentInstallationResourceOwner(fence.OwnerReferences, controllerOwnerReference(corev1.SchemeGroupVersion.String(), "ConfigMap", intentAnchor.Name, intentAnchor.UID)))

	service := &corev1.Service{}
	require.NoError(t, kubeClient.Get(context.Background(), types.NamespacedName{Namespace: podNamespace, Name: uninstallFenceWebhookServiceName}, service))
	require.Equal(t, int32(uninstallFenceWebhookPort), service.Spec.Ports[0].TargetPort.IntVal)

	configuration := &admissionregistrationv1.ValidatingWebhookConfiguration{}
	require.NoError(t, kubeClient.Get(context.Background(), types.NamespacedName{Name: uninstallFenceWebhookConfigurationName}, configuration))
	require.Len(t, configuration.Webhooks, 1)
	require.Equal(t, admissionregistrationv1.Ignore, *configuration.Webhooks[0].FailurePolicy)

	originalCertificate := append([]byte(nil), secret.Data[uninstallFenceWebhookCertificateName]...)
	fence.Data[uninstallFenceStateKey] = uninstallFenceStateActive
	require.NoError(t, kubeClient.Update(context.Background(), fence))
	require.NoError(t, PrepareUninstallFenceWebhook(context.Background(), kubeClient, podNamespace))
	require.NoError(t, kubeClient.Get(context.Background(), types.NamespacedName{Namespace: podNamespace, Name: uninstallFenceWebhookTLSSecretName}, secret))
	require.Equal(t, originalCertificate, secret.Data[uninstallFenceWebhookCertificateName])
	require.NoError(t, kubeClient.Get(context.Background(), types.NamespacedName{Name: uninstallFenceWebhookConfigurationName}, configuration))
	require.Equal(t, admissionregistrationv1.Fail, *configuration.Webhooks[0].FailurePolicy)
	require.NoError(t, validateUninstallFenceWebhook(&configuration.Webhooks[0], podNamespace))
}

func TestPrepareUninstallFenceWebhookRejectsForeignTLSSecret(t *testing.T) {
	const podNamespace = "datadog-agent"
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, rbacv1.AddToScheme(scheme))
	require.NoError(t, admissionregistrationv1.AddToScheme(scheme))

	kubeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Namespace: podNamespace, Name: managedAgentInstallationRuntimeAnchorName, UID: types.UID("runtime-anchor-uid")}},
		&rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: managedAgentInstallationRuntimeAnchorName, UID: types.UID("cluster-anchor-uid")}},
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Namespace: managedAgentInstallationIntentKey.Namespace, Name: managedAgentInstallationIntentKey.Name, UID: types.UID("intent-anchor-uid")}},
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Namespace: podNamespace, Name: uninstallFenceWebhookTLSSecretName}},
	).Build()

	oldCertDir := UninstallFenceAdmissionCertDir
	UninstallFenceAdmissionCertDir = t.TempDir()
	t.Cleanup(func() { UninstallFenceAdmissionCertDir = oldCertDir })

	err := PrepareUninstallFenceWebhook(context.Background(), kubeClient, podNamespace)
	require.ErrorContains(t, err, "TLS Secret ownership")
}

func TestPrepareUninstallFenceWebhookRejectsForeignFence(t *testing.T) {
	const podNamespace = "datadog-agent"
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, rbacv1.AddToScheme(scheme))
	require.NoError(t, admissionregistrationv1.AddToScheme(scheme))

	kubeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Namespace: podNamespace, Name: managedAgentInstallationRuntimeAnchorName, UID: types.UID("runtime-anchor-uid")}},
		&rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: managedAgentInstallationRuntimeAnchorName, UID: types.UID("cluster-anchor-uid")}},
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Namespace: managedAgentInstallationIntentKey.Namespace, Name: managedAgentInstallationIntentKey.Name, UID: types.UID("intent-anchor-uid")}},
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Namespace: uninstallFenceKey.Namespace, Name: uninstallFenceKey.Name},
			Data:       map[string]string{uninstallFenceStateKey: uninstallFenceStateInactive},
		},
	).Build()

	err := PrepareUninstallFenceWebhook(context.Background(), kubeClient, podNamespace)
	require.ErrorContains(t, err, "uninstall fence ConfigMap ownership")
}

func TestEnsureUninstallFenceConfigMapRecoversFromConcurrentCreate(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	kubeClient := &persistThenAlreadyExistsClient{
		Client:   fake.NewClientBuilder().WithScheme(scheme).Build(),
		key:      uninstallFenceKey,
		resource: "configmaps",
	}
	intent := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{
		Namespace: managedAgentInstallationIntentKey.Namespace,
		Name:      managedAgentInstallationIntentKey.Name,
		UID:       types.UID("intent-anchor-uid"),
	}}

	fence, err := ensureUninstallFenceConfigMap(context.Background(), kubeClient, intent)
	require.NoError(t, err)
	require.Equal(t, uninstallFenceStateInactive, fence.Data[uninstallFenceStateKey])
	require.NoError(t, requireManagedAgentInstallationResourceOwner(
		fence.OwnerReferences,
		controllerOwnerReference(corev1.SchemeGroupVersion.String(), "ConfigMap", intent.Name, intent.UID),
	))
}

func TestEnsureUninstallFenceWebhookCertificateRecoversFromConcurrentCreate(t *testing.T) {
	const podNamespace = "datadog-agent"
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	key := types.NamespacedName{Namespace: podNamespace, Name: uninstallFenceWebhookTLSSecretName}
	kubeClient := &persistThenAlreadyExistsClient{
		Client:   fake.NewClientBuilder().WithScheme(scheme).Build(),
		key:      key,
		resource: "secrets",
	}
	anchor := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{
		Namespace: podNamespace,
		Name:      managedAgentInstallationRuntimeAnchorName,
		UID:       types.UID("runtime-anchor-uid"),
	}}
	now := metav1.Now().Time

	certificate, err := ensureUninstallFenceWebhookCertificate(context.Background(), kubeClient, podNamespace, anchor, now)
	require.NoError(t, err)
	require.NoError(t, validateUninstallFenceWebhookCertificate(certificate, podNamespace, now))
	persisted := &corev1.Secret{}
	require.NoError(t, kubeClient.Get(context.Background(), key, persisted))
	require.Equal(t, persisted.Data, uninstallFenceWebhookCertificateData(certificate))
}
