// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fleet

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"time"

	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	v2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
)

const (
	lifecycleRuntimeAnchorName      = "datadog-agent-lifecycle-runtime"
	lifecycleWebhookTLSSecretName   = "datadog-agent-lifecycle-webhook-tls"
	lifecycleWebhookPort            = 9443
	lifecycleWebhookCertValidity    = 5 * 365 * 24 * time.Hour
	lifecycleWebhookRenewBefore     = 30 * 24 * time.Hour
	lifecycleWebhookCertificateName = "tls.crt"
	lifecycleWebhookPrivateKeyName  = "tls.key"
	lifecycleWebhookCAName          = "ca.crt"
)

// LifecycleAdmissionCertDir is controller-runtime's default webhook certificate directory.
var LifecycleAdmissionCertDir = filepath.Join(os.TempDir(), "k8s-webhook-server", "serving-certs")

// PrepareLifecycleAdmissionWebhook ensures the TLS identity and Kubernetes resources used by
// the uninstall fence exist before the controller-runtime webhook server starts.
func PrepareLifecycleAdmissionWebhook(ctx context.Context, kubeClient client.Client, podNamespace string) error {
	if podNamespace == "" {
		return fmt.Errorf("POD_NAMESPACE is required for EKS lifecycle admission")
	}

	anchor, clusterAnchor, err := readLifecycleRuntimeAnchors(ctx, kubeClient, podNamespace)
	if err != nil {
		return err
	}
	intent, err := readLifecycleIntentAnchor(ctx, kubeClient)
	if err != nil {
		return err
	}
	fence, err := ensureUninstallFenceConfigMap(ctx, kubeClient, intent)
	if err != nil {
		return err
	}

	certificate, err := ensureLifecycleWebhookCertificate(ctx, kubeClient, podNamespace, anchor, time.Now())
	if err != nil {
		return err
	}
	if err := writeLifecycleWebhookCertificateFiles(LifecycleAdmissionCertDir, certificate); err != nil {
		return err
	}
	if err := ensureLifecycleWebhookService(ctx, kubeClient, podNamespace, anchor); err != nil {
		return err
	}
	return ensureLifecycleWebhookConfiguration(ctx, kubeClient, podNamespace, clusterAnchor, certificate.caCertificate, fence.Data[uninstallFenceStateKey] == uninstallFenceStateActive)
}

type lifecycleWebhookCertificate struct {
	serverCertificate []byte
	privateKey        []byte
	caCertificate     []byte
}

func readLifecycleRuntimeAnchors(ctx context.Context, kubeClient client.Client, namespace string) (*corev1.ConfigMap, *rbacv1.ClusterRole, error) {
	anchor := &corev1.ConfigMap{}
	if err := kubeClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: lifecycleRuntimeAnchorName}, anchor); err != nil {
		return nil, nil, fmt.Errorf("read lifecycle runtime ConfigMap anchor: %w", err)
	}
	clusterAnchor := &rbacv1.ClusterRole{}
	if err := kubeClient.Get(ctx, types.NamespacedName{Name: lifecycleRuntimeAnchorName}, clusterAnchor); err != nil {
		return nil, nil, fmt.Errorf("read lifecycle runtime ClusterRole anchor: %w", err)
	}
	return anchor, clusterAnchor, nil
}

func readLifecycleIntentAnchor(ctx context.Context, kubeClient client.Client) (*corev1.ConfigMap, error) {
	intent := &corev1.ConfigMap{}
	if err := kubeClient.Get(ctx, addonLifecycleIntentKey, intent); err != nil {
		return nil, fmt.Errorf("read lifecycle intent ConfigMap anchor: %w", err)
	}
	return intent, nil
}

func controllerOwnerReference(apiVersion, kind, name string, uid types.UID) metav1.OwnerReference {
	return metav1.OwnerReference{
		APIVersion:         apiVersion,
		Kind:               kind,
		Name:               name,
		UID:                uid,
		Controller:         ptr.To(true),
		BlockOwnerDeletion: ptr.To(true),
	}
}

func ensureUninstallFenceConfigMap(ctx context.Context, kubeClient client.Client, intent *corev1.ConfigMap) (*corev1.ConfigMap, error) {
	wantOwner := controllerOwnerReference(corev1.SchemeGroupVersion.String(), "ConfigMap", intent.Name, intent.UID)
	fence := &corev1.ConfigMap{}
	err := kubeClient.Get(ctx, uninstallFenceKey, fence)
	if apierrors.IsNotFound(err) {
		fence = &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:       uninstallFenceKey.Namespace,
				Name:            uninstallFenceKey.Name,
				OwnerReferences: []metav1.OwnerReference{wantOwner},
				Labels:          map[string]string{"app.kubernetes.io/managed-by": "datadog-operator"},
			},
			Data: map[string]string{uninstallFenceStateKey: uninstallFenceStateInactive},
		}
		if err := kubeClient.Create(ctx, fence, client.FieldOwner("fleet-daemon")); err != nil {
			return nil, fmt.Errorf("create uninstall fence ConfigMap: %w", err)
		}
		return fence, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read uninstall fence ConfigMap: %w", err)
	}
	if err := requireLifecycleResourceOwner(fence.OwnerReferences, wantOwner); err != nil {
		return nil, fmt.Errorf("validate uninstall fence ConfigMap ownership: %w", err)
	}
	return fence, nil
}

func ensureLifecycleWebhookCertificate(ctx context.Context, kubeClient client.Client, namespace string, anchor *corev1.ConfigMap, now time.Time) (*lifecycleWebhookCertificate, error) {
	wantOwner := controllerOwnerReference(corev1.SchemeGroupVersion.String(), "ConfigMap", anchor.Name, anchor.UID)
	secret := &corev1.Secret{}
	err := kubeClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: lifecycleWebhookTLSSecretName}, secret)
	secretExists := err == nil
	if err == nil {
		if err := requireLifecycleResourceOwner(secret.OwnerReferences, wantOwner); err != nil {
			return nil, fmt.Errorf("validate lifecycle webhook TLS Secret ownership: %w", err)
		}
		certificate := lifecycleWebhookCertificateFromSecret(secret)
		if err := validateLifecycleWebhookCertificate(certificate, namespace, now); err == nil {
			return certificate, nil
		}
	}
	if err != nil && !apierrors.IsNotFound(err) {
		return nil, fmt.Errorf("read lifecycle webhook TLS Secret: %w", err)
	}

	certificate, err := generateLifecycleWebhookCertificate(namespace, now)
	if err != nil {
		return nil, err
	}
	if !secretExists {
		secret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:       namespace,
				Name:            lifecycleWebhookTLSSecretName,
				OwnerReferences: []metav1.OwnerReference{wantOwner},
				Labels:          map[string]string{"app.kubernetes.io/managed-by": "datadog-operator"},
			},
			Type: corev1.SecretTypeTLS,
		}
		secret.Data = lifecycleWebhookCertificateData(certificate)
		if err := kubeClient.Create(ctx, secret, client.FieldOwner("fleet-daemon")); err != nil {
			return nil, fmt.Errorf("create lifecycle webhook TLS Secret: %w", err)
		}
		return certificate, nil
	}

	base := secret.DeepCopy()
	secret.Type = corev1.SecretTypeTLS
	secret.Data = lifecycleWebhookCertificateData(certificate)
	if err := kubeClient.Patch(ctx, secret, client.MergeFrom(base), client.FieldOwner("fleet-daemon")); err != nil {
		return nil, fmt.Errorf("rotate lifecycle webhook TLS Secret: %w", err)
	}
	return certificate, nil
}

func lifecycleWebhookCertificateFromSecret(secret *corev1.Secret) *lifecycleWebhookCertificate {
	return &lifecycleWebhookCertificate{
		serverCertificate: secret.Data[lifecycleWebhookCertificateName],
		privateKey:        secret.Data[lifecycleWebhookPrivateKeyName],
		caCertificate:     secret.Data[lifecycleWebhookCAName],
	}
}

func lifecycleWebhookCertificateData(certificate *lifecycleWebhookCertificate) map[string][]byte {
	return map[string][]byte{
		lifecycleWebhookCertificateName: certificate.serverCertificate,
		lifecycleWebhookPrivateKeyName:  certificate.privateKey,
		lifecycleWebhookCAName:          certificate.caCertificate,
	}
}

func validateLifecycleWebhookCertificate(certificate *lifecycleWebhookCertificate, namespace string, now time.Time) error {
	keyPair, err := tls.X509KeyPair(certificate.serverCertificate, certificate.privateKey)
	if err != nil {
		return err
	}
	if len(keyPair.Certificate) == 0 {
		return fmt.Errorf("lifecycle webhook certificate chain is empty")
	}
	server, err := x509.ParseCertificate(keyPair.Certificate[0])
	if err != nil {
		return err
	}
	caBlock, _ := pem.Decode(certificate.caCertificate)
	if caBlock == nil {
		return fmt.Errorf("decode lifecycle webhook CA certificate")
	}
	ca, err := x509.ParseCertificate(caBlock.Bytes)
	if err != nil {
		return err
	}
	roots := x509.NewCertPool()
	roots.AddCert(ca)
	_, err = server.Verify(x509.VerifyOptions{
		DNSName:     lifecycleWebhookDNSNames(namespace)[0],
		Roots:       roots,
		CurrentTime: now,
		KeyUsages:   []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	})
	if err != nil {
		return err
	}
	if server.NotAfter.Before(now.Add(lifecycleWebhookRenewBefore)) {
		return fmt.Errorf("lifecycle webhook certificate expires too soon")
	}
	return nil
}

func generateLifecycleWebhookCertificate(namespace string, now time.Time) (*lifecycleWebhookCertificate, error) {
	caKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("generate lifecycle webhook CA key: %w", err)
	}
	serialLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	caSerial, err := rand.Int(rand.Reader, serialLimit)
	if err != nil {
		return nil, fmt.Errorf("generate lifecycle webhook CA serial: %w", err)
	}
	caTemplate := &x509.Certificate{
		SerialNumber:          caSerial,
		Subject:               pkix.Name{CommonName: "Datadog Operator lifecycle webhook CA"},
		NotBefore:             now.Add(-5 * time.Minute),
		NotAfter:              now.Add(lifecycleWebhookCertValidity),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		return nil, fmt.Errorf("create lifecycle webhook CA certificate: %w", err)
	}

	serverKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("generate lifecycle webhook server key: %w", err)
	}
	serverSerial, err := rand.Int(rand.Reader, serialLimit)
	if err != nil {
		return nil, fmt.Errorf("generate lifecycle webhook server serial: %w", err)
	}
	dnsNames := lifecycleWebhookDNSNames(namespace)
	serverTemplate := &x509.Certificate{
		SerialNumber: serverSerial,
		Subject:      pkix.Name{CommonName: dnsNames[0]},
		NotBefore:    now.Add(-5 * time.Minute),
		NotAfter:     now.Add(lifecycleWebhookCertValidity),
		DNSNames:     dnsNames,
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	serverDER, err := x509.CreateCertificate(rand.Reader, serverTemplate, caTemplate, &serverKey.PublicKey, caKey)
	if err != nil {
		return nil, fmt.Errorf("create lifecycle webhook server certificate: %w", err)
	}
	serverKeyDER, err := x509.MarshalPKCS8PrivateKey(serverKey)
	if err != nil {
		return nil, fmt.Errorf("marshal lifecycle webhook server key: %w", err)
	}
	return &lifecycleWebhookCertificate{
		serverCertificate: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: serverDER}),
		privateKey:        pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: serverKeyDER}),
		caCertificate:     pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER}),
	}, nil
}

func lifecycleWebhookDNSNames(namespace string) []string {
	return []string{
		uninstallFenceWebhookServiceName + "." + namespace + ".svc",
		uninstallFenceWebhookServiceName + "." + namespace + ".svc.cluster.local",
	}
}

func writeLifecycleWebhookCertificateFiles(directory string, certificate *lifecycleWebhookCertificate) error {
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create lifecycle webhook certificate directory: %w", err)
	}
	if err := os.WriteFile(filepath.Join(directory, lifecycleWebhookCertificateName), certificate.serverCertificate, 0o600); err != nil {
		return fmt.Errorf("write lifecycle webhook certificate: %w", err)
	}
	if err := os.WriteFile(filepath.Join(directory, lifecycleWebhookPrivateKeyName), certificate.privateKey, 0o600); err != nil {
		return fmt.Errorf("write lifecycle webhook private key: %w", err)
	}
	return nil
}

func ensureLifecycleWebhookService(ctx context.Context, kubeClient client.Client, namespace string, anchor *corev1.ConfigMap) error {
	wantOwner := controllerOwnerReference(corev1.SchemeGroupVersion.String(), "ConfigMap", anchor.Name, anchor.UID)
	wantSpec := corev1.ServiceSpec{
		Selector: map[string]string{"app.kubernetes.io/name": "datadog-operator"},
		Ports: []corev1.ServicePort{{
			Name:       "webhook",
			Port:       443,
			TargetPort: intstr.FromInt32(lifecycleWebhookPort),
		}},
	}
	service := &corev1.Service{}
	err := kubeClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: uninstallFenceWebhookServiceName}, service)
	if apierrors.IsNotFound(err) {
		return kubeClient.Create(ctx, &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:       namespace,
				Name:            uninstallFenceWebhookServiceName,
				OwnerReferences: []metav1.OwnerReference{wantOwner},
				Labels:          map[string]string{"app.kubernetes.io/managed-by": "datadog-operator"},
			},
			Spec: wantSpec,
		}, client.FieldOwner("fleet-daemon"))
	}
	if err != nil {
		return fmt.Errorf("read lifecycle webhook Service: %w", err)
	}
	if err := requireLifecycleResourceOwner(service.OwnerReferences, wantOwner); err != nil {
		return fmt.Errorf("validate lifecycle webhook Service ownership: %w", err)
	}
	base := service.DeepCopy()
	wantSpec.ClusterIP = service.Spec.ClusterIP
	wantSpec.ClusterIPs = service.Spec.ClusterIPs
	wantSpec.IPFamilies = service.Spec.IPFamilies
	wantSpec.IPFamilyPolicy = service.Spec.IPFamilyPolicy
	wantSpec.InternalTrafficPolicy = service.Spec.InternalTrafficPolicy
	wantSpec.SessionAffinity = service.Spec.SessionAffinity
	service.Spec = wantSpec
	return kubeClient.Patch(ctx, service, client.MergeFrom(base), client.FieldOwner("fleet-daemon"))
}

func ensureLifecycleWebhookConfiguration(ctx context.Context, kubeClient client.Client, namespace string, anchor *rbacv1.ClusterRole, caCertificate []byte, active bool) error {
	wantOwner := controllerOwnerReference(rbacv1.SchemeGroupVersion.String(), "ClusterRole", anchor.Name, anchor.UID)
	failurePolicy := admissionregistrationv1.Ignore
	if active {
		failurePolicy = admissionregistrationv1.Fail
	}
	path := uninstallFenceAdmissionPath
	port := int32(443)
	wantWebhooks := []admissionregistrationv1.ValidatingWebhook{{
		Name: uninstallFenceWebhookName,
		ClientConfig: admissionregistrationv1.WebhookClientConfig{
			CABundle: caCertificate,
			Service: &admissionregistrationv1.ServiceReference{
				Namespace: namespace,
				Name:      uninstallFenceWebhookServiceName,
				Path:      &path,
				Port:      &port,
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
		FailurePolicy:           &failurePolicy,
		SideEffects:             ptr.To(admissionregistrationv1.SideEffectClassNone),
		AdmissionReviewVersions: []string{"v1"},
		TimeoutSeconds:          ptr.To[int32](5),
	}}
	configuration := &admissionregistrationv1.ValidatingWebhookConfiguration{}
	err := kubeClient.Get(ctx, types.NamespacedName{Name: uninstallFenceWebhookConfigurationName}, configuration)
	if apierrors.IsNotFound(err) {
		return kubeClient.Create(ctx, &admissionregistrationv1.ValidatingWebhookConfiguration{
			ObjectMeta: metav1.ObjectMeta{
				Name:            uninstallFenceWebhookConfigurationName,
				OwnerReferences: []metav1.OwnerReference{wantOwner},
				Labels:          map[string]string{"app.kubernetes.io/managed-by": "datadog-operator"},
			},
			Webhooks: wantWebhooks,
		}, client.FieldOwner("fleet-daemon"))
	}
	if err != nil {
		return fmt.Errorf("read lifecycle ValidatingWebhookConfiguration: %w", err)
	}
	if err := requireLifecycleResourceOwner(configuration.OwnerReferences, wantOwner); err != nil {
		return fmt.Errorf("validate lifecycle ValidatingWebhookConfiguration ownership: %w", err)
	}
	base := configuration.DeepCopy()
	configuration.Webhooks = wantWebhooks
	return kubeClient.Patch(ctx, configuration, client.MergeFrom(base), client.FieldOwner("fleet-daemon"))
}

func requireLifecycleResourceOwner(owners []metav1.OwnerReference, want metav1.OwnerReference) error {
	for _, owner := range owners {
		if owner.APIVersion == want.APIVersion && owner.Kind == want.Kind && owner.Name == want.Name && owner.UID == want.UID && owner.Controller != nil && *owner.Controller {
			return nil
		}
	}
	return fmt.Errorf("resource is not controlled by %s %s with UID %s", want.Kind, want.Name, want.UID)
}
