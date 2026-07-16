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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestPrepareLifecycleAdmissionWebhook(t *testing.T) {
	const podNamespace = "datadog-agent"
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, rbacv1.AddToScheme(scheme))
	require.NoError(t, admissionregistrationv1.AddToScheme(scheme))

	runtimeAnchor := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{
		Namespace: podNamespace,
		Name:      lifecycleRuntimeAnchorName,
		UID:       types.UID("runtime-anchor-uid"),
	}}
	clusterAnchor := &rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{
		Name: lifecycleRuntimeAnchorName,
		UID:  types.UID("cluster-anchor-uid"),
	}}
	intentAnchor := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{
		Namespace: addonLifecycleIntentKey.Namespace,
		Name:      addonLifecycleIntentKey.Name,
		UID:       types.UID("intent-anchor-uid"),
	}}
	kubeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(runtimeAnchor, clusterAnchor, intentAnchor).Build()

	oldCertDir := LifecycleAdmissionCertDir
	LifecycleAdmissionCertDir = t.TempDir()
	t.Cleanup(func() { LifecycleAdmissionCertDir = oldCertDir })

	require.NoError(t, PrepareLifecycleAdmissionWebhook(context.Background(), kubeClient, podNamespace))

	secret := &corev1.Secret{}
	require.NoError(t, kubeClient.Get(context.Background(), types.NamespacedName{Namespace: podNamespace, Name: lifecycleWebhookTLSSecretName}, secret))
	require.NoError(t, validateLifecycleWebhookCertificate(lifecycleWebhookCertificateFromSecret(secret), podNamespace, metav1.Now().Time))
	serverCertificate, err := os.ReadFile(filepath.Join(LifecycleAdmissionCertDir, lifecycleWebhookCertificateName))
	require.NoError(t, err)
	require.Equal(t, secret.Data[lifecycleWebhookCertificateName], serverCertificate)

	fence := &corev1.ConfigMap{}
	require.NoError(t, kubeClient.Get(context.Background(), uninstallFenceKey, fence))
	require.Equal(t, uninstallFenceStateInactive, fence.Data[uninstallFenceStateKey])
	require.NoError(t, requireLifecycleResourceOwner(fence.OwnerReferences, controllerOwnerReference(corev1.SchemeGroupVersion.String(), "ConfigMap", intentAnchor.Name, intentAnchor.UID)))

	service := &corev1.Service{}
	require.NoError(t, kubeClient.Get(context.Background(), types.NamespacedName{Namespace: podNamespace, Name: uninstallFenceWebhookServiceName}, service))
	require.Equal(t, int32(lifecycleWebhookPort), service.Spec.Ports[0].TargetPort.IntVal)

	configuration := &admissionregistrationv1.ValidatingWebhookConfiguration{}
	require.NoError(t, kubeClient.Get(context.Background(), types.NamespacedName{Name: uninstallFenceWebhookConfigurationName}, configuration))
	require.Len(t, configuration.Webhooks, 1)
	require.Equal(t, admissionregistrationv1.Ignore, *configuration.Webhooks[0].FailurePolicy)

	originalCertificate := append([]byte(nil), secret.Data[lifecycleWebhookCertificateName]...)
	fence.Data[uninstallFenceStateKey] = uninstallFenceStateActive
	require.NoError(t, kubeClient.Update(context.Background(), fence))
	require.NoError(t, PrepareLifecycleAdmissionWebhook(context.Background(), kubeClient, podNamespace))
	require.NoError(t, kubeClient.Get(context.Background(), types.NamespacedName{Namespace: podNamespace, Name: lifecycleWebhookTLSSecretName}, secret))
	require.Equal(t, originalCertificate, secret.Data[lifecycleWebhookCertificateName])
	require.NoError(t, kubeClient.Get(context.Background(), types.NamespacedName{Name: uninstallFenceWebhookConfigurationName}, configuration))
	require.Equal(t, admissionregistrationv1.Fail, *configuration.Webhooks[0].FailurePolicy)
	require.NoError(t, validateUninstallFenceWebhook(&configuration.Webhooks[0], podNamespace))
}

func TestPrepareLifecycleAdmissionWebhookRejectsForeignTLSSecret(t *testing.T) {
	const podNamespace = "datadog-agent"
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, rbacv1.AddToScheme(scheme))
	require.NoError(t, admissionregistrationv1.AddToScheme(scheme))

	kubeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Namespace: podNamespace, Name: lifecycleRuntimeAnchorName, UID: types.UID("runtime-anchor-uid")}},
		&rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: lifecycleRuntimeAnchorName, UID: types.UID("cluster-anchor-uid")}},
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Namespace: addonLifecycleIntentKey.Namespace, Name: addonLifecycleIntentKey.Name, UID: types.UID("intent-anchor-uid")}},
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Namespace: podNamespace, Name: lifecycleWebhookTLSSecretName}},
	).Build()

	oldCertDir := LifecycleAdmissionCertDir
	LifecycleAdmissionCertDir = t.TempDir()
	t.Cleanup(func() { LifecycleAdmissionCertDir = oldCertDir })

	err := PrepareLifecycleAdmissionWebhook(context.Background(), kubeClient, podNamespace)
	require.ErrorContains(t, err, "TLS Secret ownership")
}

func TestPrepareLifecycleAdmissionWebhookRejectsForeignFence(t *testing.T) {
	const podNamespace = "datadog-agent"
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, rbacv1.AddToScheme(scheme))
	require.NoError(t, admissionregistrationv1.AddToScheme(scheme))

	kubeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Namespace: podNamespace, Name: lifecycleRuntimeAnchorName, UID: types.UID("runtime-anchor-uid")}},
		&rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: lifecycleRuntimeAnchorName, UID: types.UID("cluster-anchor-uid")}},
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Namespace: addonLifecycleIntentKey.Namespace, Name: addonLifecycleIntentKey.Name, UID: types.UID("intent-anchor-uid")}},
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Namespace: uninstallFenceKey.Namespace, Name: uninstallFenceKey.Name},
			Data:       map[string]string{uninstallFenceStateKey: uninstallFenceStateInactive},
		},
	).Build()

	err := PrepareLifecycleAdmissionWebhook(context.Background(), kubeClient, podNamespace)
	require.ErrorContains(t, err, "uninstall fence ConfigMap ownership")
}
