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
	managedAgentInstallationRuntimeAnchorName = "datadog-agent-managed-installation-runtime"
	uninstallFenceWebhookTLSSecretName        = "datadog-agent-uninstall-fence-webhook-tls"
	uninstallFenceWebhookPort                 = 9443
	uninstallFenceWebhookCertValidity         = 5 * 365 * 24 * time.Hour
	uninstallFenceWebhookRenewBefore          = 30 * 24 * time.Hour
	uninstallFenceWebhookCertificateName      = "tls.crt"
	uninstallFenceWebhookPrivateKeyName       = "tls.key"
	uninstallFenceWebhookCAName               = "ca.crt"
)

// UninstallFenceAdmissionCertDir is controller-runtime's default webhook certificate directory.
var UninstallFenceAdmissionCertDir = filepath.Join(os.TempDir(), "k8s-webhook-server", "serving-certs")

// PrepareUninstallFenceWebhook ensures the TLS identity and Kubernetes resources used by
// the uninstall fence exist before the controller-runtime webhook server starts.
func PrepareUninstallFenceWebhook(ctx context.Context, kubeClient client.Client, podNamespace string) error {
	if podNamespace == "" {
		return fmt.Errorf("POD_NAMESPACE is required for EKS managed Agent installation admission")
	}

	anchor, clusterAnchor, err := readManagedAgentInstallationRuntimeAnchors(ctx, kubeClient, podNamespace)
	if err != nil {
		return err
	}
	intent, err := readManagedAgentInstallationIntentAnchor(ctx, kubeClient)
	if err != nil {
		return err
	}
	fence, err := ensureUninstallFenceConfigMap(ctx, kubeClient, intent)
	if err != nil {
		return err
	}

	certificate, err := ensureUninstallFenceWebhookCertificate(ctx, kubeClient, podNamespace, anchor, time.Now())
	if err != nil {
		return err
	}
	if err := writeUninstallFenceWebhookCertificateFiles(UninstallFenceAdmissionCertDir, certificate); err != nil {
		return err
	}
	if err := ensureUninstallFenceWebhookService(ctx, kubeClient, podNamespace, anchor); err != nil {
		return err
	}
	return ensureUninstallFenceWebhookConfiguration(ctx, kubeClient, podNamespace, clusterAnchor, certificate.caCertificate, fence.Data[uninstallFenceStateKey] == uninstallFenceStateActive)
}

type uninstallFenceWebhookCertificate struct {
	serverCertificate []byte
	privateKey        []byte
	caCertificate     []byte
}

func readManagedAgentInstallationRuntimeAnchors(ctx context.Context, kubeClient client.Client, namespace string) (*corev1.ConfigMap, *rbacv1.ClusterRole, error) {
	anchor := &corev1.ConfigMap{}
	if err := kubeClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: managedAgentInstallationRuntimeAnchorName}, anchor); err != nil {
		return nil, nil, fmt.Errorf("read managed Agent installation runtime ConfigMap anchor: %w", err)
	}
	clusterAnchor := &rbacv1.ClusterRole{}
	if err := kubeClient.Get(ctx, types.NamespacedName{Name: managedAgentInstallationRuntimeAnchorName}, clusterAnchor); err != nil {
		return nil, nil, fmt.Errorf("read managed Agent installation runtime ClusterRole anchor: %w", err)
	}
	return anchor, clusterAnchor, nil
}

func readManagedAgentInstallationIntentAnchor(ctx context.Context, kubeClient client.Client) (*corev1.ConfigMap, error) {
	intent := &corev1.ConfigMap{}
	if err := kubeClient.Get(ctx, managedAgentInstallationIntentKey, intent); err != nil {
		return nil, fmt.Errorf("read managed Agent installation intent ConfigMap anchor: %w", err)
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
	getErr := kubeClient.Get(ctx, uninstallFenceKey, fence)
	if apierrors.IsNotFound(getErr) {
		fence = &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:       uninstallFenceKey.Namespace,
				Name:            uninstallFenceKey.Name,
				OwnerReferences: []metav1.OwnerReference{wantOwner},
				Labels:          map[string]string{"app.kubernetes.io/managed-by": "datadog-operator"},
			},
			Data: map[string]string{uninstallFenceStateKey: uninstallFenceStateInactive},
		}
		if createErr := kubeClient.Create(ctx, fence, client.FieldOwner("fleet-daemon")); createErr != nil {
			return nil, fmt.Errorf("create uninstall fence ConfigMap: %w", createErr)
		}
		return fence, nil
	}
	if getErr != nil {
		return nil, fmt.Errorf("read uninstall fence ConfigMap: %w", getErr)
	}
	if err := requireManagedAgentInstallationResourceOwner(fence.OwnerReferences, wantOwner); err != nil {
		return nil, fmt.Errorf("validate uninstall fence ConfigMap ownership: %w", err)
	}
	return fence, nil
}

func ensureUninstallFenceWebhookCertificate(ctx context.Context, kubeClient client.Client, namespace string, anchor *corev1.ConfigMap, now time.Time) (*uninstallFenceWebhookCertificate, error) {
	wantOwner := controllerOwnerReference(corev1.SchemeGroupVersion.String(), "ConfigMap", anchor.Name, anchor.UID)
	secret := &corev1.Secret{}
	getErr := kubeClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: uninstallFenceWebhookTLSSecretName}, secret)
	secretExists := getErr == nil
	if getErr == nil {
		if err := requireManagedAgentInstallationResourceOwner(secret.OwnerReferences, wantOwner); err != nil {
			return nil, fmt.Errorf("validate managed Agent installation webhook TLS Secret ownership: %w", err)
		}
		certificate := uninstallFenceWebhookCertificateFromSecret(secret)
		if err := validateUninstallFenceWebhookCertificate(certificate, namespace, now); err == nil {
			return certificate, nil
		}
	}
	if getErr != nil && !apierrors.IsNotFound(getErr) {
		return nil, fmt.Errorf("read managed Agent installation webhook TLS Secret: %w", getErr)
	}

	certificate, certificateErr := generateUninstallFenceWebhookCertificate(namespace, now)
	if certificateErr != nil {
		return nil, certificateErr
	}
	if !secretExists {
		secret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:       namespace,
				Name:            uninstallFenceWebhookTLSSecretName,
				OwnerReferences: []metav1.OwnerReference{wantOwner},
				Labels:          map[string]string{"app.kubernetes.io/managed-by": "datadog-operator"},
			},
			Type: corev1.SecretTypeTLS,
		}
		secret.Data = uninstallFenceWebhookCertificateData(certificate)
		if err := kubeClient.Create(ctx, secret, client.FieldOwner("fleet-daemon")); err != nil {
			return nil, fmt.Errorf("create managed Agent installation webhook TLS Secret: %w", err)
		}
		return certificate, nil
	}

	base := secret.DeepCopy()
	secret.Type = corev1.SecretTypeTLS
	secret.Data = uninstallFenceWebhookCertificateData(certificate)
	if err := kubeClient.Patch(ctx, secret, client.MergeFrom(base), client.FieldOwner("fleet-daemon")); err != nil {
		return nil, fmt.Errorf("rotate managed Agent installation webhook TLS Secret: %w", err)
	}
	return certificate, nil
}

func uninstallFenceWebhookCertificateFromSecret(secret *corev1.Secret) *uninstallFenceWebhookCertificate {
	return &uninstallFenceWebhookCertificate{
		serverCertificate: secret.Data[uninstallFenceWebhookCertificateName],
		privateKey:        secret.Data[uninstallFenceWebhookPrivateKeyName],
		caCertificate:     secret.Data[uninstallFenceWebhookCAName],
	}
}

func uninstallFenceWebhookCertificateData(certificate *uninstallFenceWebhookCertificate) map[string][]byte {
	return map[string][]byte{
		uninstallFenceWebhookCertificateName: certificate.serverCertificate,
		uninstallFenceWebhookPrivateKeyName:  certificate.privateKey,
		uninstallFenceWebhookCAName:          certificate.caCertificate,
	}
}

func validateUninstallFenceWebhookCertificate(certificate *uninstallFenceWebhookCertificate, namespace string, now time.Time) error {
	keyPair, err := tls.X509KeyPair(certificate.serverCertificate, certificate.privateKey)
	if err != nil {
		return err
	}
	if len(keyPair.Certificate) == 0 {
		return fmt.Errorf("managed Agent installation webhook certificate chain is empty")
	}
	server, err := x509.ParseCertificate(keyPair.Certificate[0])
	if err != nil {
		return err
	}
	caBlock, _ := pem.Decode(certificate.caCertificate)
	if caBlock == nil {
		return fmt.Errorf("decode managed Agent installation webhook CA certificate")
	}
	ca, err := x509.ParseCertificate(caBlock.Bytes)
	if err != nil {
		return err
	}
	roots := x509.NewCertPool()
	roots.AddCert(ca)
	_, err = server.Verify(x509.VerifyOptions{
		DNSName:     uninstallFenceWebhookDNSNames(namespace)[0],
		Roots:       roots,
		CurrentTime: now,
		KeyUsages:   []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	})
	if err != nil {
		return err
	}
	if server.NotAfter.Before(now.Add(uninstallFenceWebhookRenewBefore)) {
		return fmt.Errorf("managed Agent installation webhook certificate expires too soon")
	}
	return nil
}

func generateUninstallFenceWebhookCertificate(namespace string, now time.Time) (*uninstallFenceWebhookCertificate, error) {
	caKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("generate managed Agent installation webhook CA key: %w", err)
	}
	serialLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	caSerial, err := rand.Int(rand.Reader, serialLimit)
	if err != nil {
		return nil, fmt.Errorf("generate managed Agent installation webhook CA serial: %w", err)
	}
	caTemplate := &x509.Certificate{
		SerialNumber:          caSerial,
		Subject:               pkix.Name{CommonName: "Datadog Operator managed Agent installation webhook CA"},
		NotBefore:             now.Add(-5 * time.Minute),
		NotAfter:              now.Add(uninstallFenceWebhookCertValidity),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		return nil, fmt.Errorf("create managed Agent installation webhook CA certificate: %w", err)
	}

	serverKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("generate managed Agent installation webhook server key: %w", err)
	}
	serverSerial, err := rand.Int(rand.Reader, serialLimit)
	if err != nil {
		return nil, fmt.Errorf("generate managed Agent installation webhook server serial: %w", err)
	}
	dnsNames := uninstallFenceWebhookDNSNames(namespace)
	serverTemplate := &x509.Certificate{
		SerialNumber: serverSerial,
		Subject:      pkix.Name{CommonName: dnsNames[0]},
		NotBefore:    now.Add(-5 * time.Minute),
		NotAfter:     now.Add(uninstallFenceWebhookCertValidity),
		DNSNames:     dnsNames,
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	serverDER, err := x509.CreateCertificate(rand.Reader, serverTemplate, caTemplate, &serverKey.PublicKey, caKey)
	if err != nil {
		return nil, fmt.Errorf("create managed Agent installation webhook server certificate: %w", err)
	}
	serverKeyDER, err := x509.MarshalPKCS8PrivateKey(serverKey)
	if err != nil {
		return nil, fmt.Errorf("marshal managed Agent installation webhook server key: %w", err)
	}
	return &uninstallFenceWebhookCertificate{
		serverCertificate: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: serverDER}),
		privateKey:        pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: serverKeyDER}),
		caCertificate:     pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER}),
	}, nil
}

func uninstallFenceWebhookDNSNames(namespace string) []string {
	return []string{
		uninstallFenceWebhookServiceName + "." + namespace + ".svc",
		uninstallFenceWebhookServiceName + "." + namespace + ".svc.cluster.local",
	}
}

func writeUninstallFenceWebhookCertificateFiles(directory string, certificate *uninstallFenceWebhookCertificate) error {
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create managed Agent installation webhook certificate directory: %w", err)
	}
	if err := os.WriteFile(filepath.Join(directory, uninstallFenceWebhookCertificateName), certificate.serverCertificate, 0o600); err != nil {
		return fmt.Errorf("write managed Agent installation webhook certificate: %w", err)
	}
	if err := os.WriteFile(filepath.Join(directory, uninstallFenceWebhookPrivateKeyName), certificate.privateKey, 0o600); err != nil {
		return fmt.Errorf("write managed Agent installation webhook private key: %w", err)
	}
	return nil
}

func ensureUninstallFenceWebhookService(ctx context.Context, kubeClient client.Client, namespace string, anchor *corev1.ConfigMap) error {
	wantOwner := controllerOwnerReference(corev1.SchemeGroupVersion.String(), "ConfigMap", anchor.Name, anchor.UID)
	wantSpec := corev1.ServiceSpec{
		Selector: map[string]string{"app.kubernetes.io/name": "datadog-operator"},
		Ports: []corev1.ServicePort{{
			Name:       "webhook",
			Port:       443,
			TargetPort: intstr.FromInt32(uninstallFenceWebhookPort),
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
		return fmt.Errorf("read managed Agent installation webhook Service: %w", err)
	}
	if err := requireManagedAgentInstallationResourceOwner(service.OwnerReferences, wantOwner); err != nil {
		return fmt.Errorf("validate managed Agent installation webhook Service ownership: %w", err)
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

func ensureUninstallFenceWebhookConfiguration(ctx context.Context, kubeClient client.Client, namespace string, anchor *rbacv1.ClusterRole, caCertificate []byte, active bool) error {
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
		return fmt.Errorf("read managed Agent installation ValidatingWebhookConfiguration: %w", err)
	}
	if err := requireManagedAgentInstallationResourceOwner(configuration.OwnerReferences, wantOwner); err != nil {
		return fmt.Errorf("validate managed Agent installation ValidatingWebhookConfiguration ownership: %w", err)
	}
	base := configuration.DeepCopy()
	configuration.Webhooks = wantWebhooks
	return kubeClient.Patch(ctx, configuration, client.MergeFrom(base), client.FieldOwner("fleet-daemon"))
}

func requireManagedAgentInstallationResourceOwner(owners []metav1.OwnerReference, want metav1.OwnerReference) error {
	for _, owner := range owners {
		if owner.APIVersion == want.APIVersion && owner.Kind == want.Kind && owner.Name == want.Name && owner.UID == want.UID && owner.Controller != nil && *owner.Controller {
			return nil
		}
	}
	return fmt.Errorf("resource is not controlled by %s %s with UID %s", want.Kind, want.Name, want.UID)
}
