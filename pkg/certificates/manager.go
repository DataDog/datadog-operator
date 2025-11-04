// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package certificates

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Manager struct {
	client client.Client
}

// NewManager creates a new certificate manager
func NewManager(client client.Client) *Manager {
	return &Manager{
		client: client,
	}
}

// GetOrCreateCA retrieves the cluster Certificate Authority (CA) if it doesn't exist
func (m *Manager) GetOrCreateCA(ctx context.Context, namespace string) (*x509.Certificate, *rsa.PrivateKey, error) {
	// Try to fetch existing CA from the DatadogAgent namespace
	secret := &corev1.Secret{}
	err := m.client.Get(ctx, types.NamespacedName{
		Name:      CASecretName,
		Namespace: namespace,
	}, secret)

	if err == nil {
		// CA exists - decode and return it
		return m.decodeCAFromSecret(secret)
	}

	// NOTE: what is the point of this? check
	if !errors.IsNotFound(err) {
		return nil, nil, fmt.Errorf("failed to check for existing CA: %w", err)
	}

	// CA doesn't exist - generate a new one
	caCert, caKey, err := m.generateCA()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate CA: %w", err)
	}

	// Store in the same namespace as DatadogAgent
	if err := m.storeCA(ctx, namespace, caCert, caKey); err != nil {
		return nil, nil, fmt.Errorf("failed to store CA: %w", err)
	}

	return caCert, caKey, nil
}

// generateCA creates a new self-signed CA certificate and private key
func (m *Manager) generateCA() (*x509.Certificate, *rsa.PrivateKey, error) {
	// Generate RSA private key (4096 buts for strong security )
	privateKey, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate RSA key: %w", err)
	}

	// Create certificate template
	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate serial number: %w", err)
	}

	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName:   "Datadog Agent Cluster CA",
			Organization: []string{"Datadog"},
		},
		NotBefore: now,
		NotAfter:  now.AddDate(CAValidityYears, 0, 0),
		// NOTE: What does this do?
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            0,
	}

	// Self-sign the certificate (CA signs itself)
	// NOTE: what is a DER, why does it need to to be created as DER and parsed?
	certDER, err := x509.CreateCertificate(
		rand.Reader,
		template,
		template, // Self-signed: parent is same as template
		&privateKey.PublicKey,
		privateKey,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create CA certificate: %w", err)
	}

	// Parse the DER-encoded certificate back to x509.Certificate
	caCert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create CA certificate: %w", err)
	}

	return caCert, privateKey, nil
}

// storeCA saves the CA certificate and private key to a kubernetes Secret
func (m *Manager) storeCA(ctx context.Context, namespace string, caCert *x509.Certificate, caKey *rsa.PrivateKey) error {
	// Encode certificate to PEM format
	// NOTE: why this particular format? Just choose one and stick with it?
	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: caCert.Raw,
	})

	// Encode private key to PEM format
	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(caKey),
	})

	// Create Kubernetes Secret with encoding
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      CASecretName,
			Namespace: namespace,
		},
		Type: corev1.SecretTypeTLS,
		Data: map[string][]byte{
			corev1.TLSCertKey:       certPEM, // "tls.crt"
			corev1.TLSPrivateKeyKey: keyPEM,  // "tls.key"
		},
	}

	return m.client.Create(ctx, secret)
}

// decodeCAFromSecret extracts the CA certificate and key from a Secret
func (m *Manager) decodeCAFromSecret(secret *corev1.Secret) (*x509.Certificate, *rsa.PrivateKey, error) {
	// Decode Certificate PEM
	certPEM := secret.Data[corev1.TLSCertKey]
	block, _ := pem.Decode(certPEM)
	if block == nil {
		return nil, nil, fmt.Errorf("failed to decode CA certificate PEM")
	}
	caCert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse CA certificate: %w", err)
	}

	// Decode private key PEM
	keyPEM := secret.Data[corev1.TLSPrivateKeyKey]
	block, _ = pem.Decode(keyPEM)
	if block == nil {
		return nil, nil, fmt.Errorf("failed to decode CA private key PEM")
	}
	caKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse CA private key: %w", err)
	}

	return caCert, caKey, nil
}

// GenerateServiceCertificate creates a new service certificate signed by the CA
// Called by operator before creating/updating a pod
// NOTE: why does it need to be generated if updating a pod?
func (m *Manager) GenerateServiceCertificate(
	ctx context.Context,
	config ServiceCertConfig,
	caCert *x509.Certificate,
	caKey *rsa.PrivateKey,
) (*corev1.Secret, error) {
	// Generate RSA private key for the service
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("failed to generate service key: %w", err)
	}

	// Create certificate template
	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, fmt.Errorf("failed to generate serial numer: %w", err)
	}

	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName:   config.CommonName,
			Organization: config.Organizations,
		},
		DNSNames:              config.DNSNames,
		NotBefore:             now,
		NotAfter:              now.AddDate(0, 0, ServiceCertValidityDays),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		IsCA:                  false,
	}

	// Sign the certificate with the CA
	certDER, err := x509.CreateCertificate(
		rand.Reader,
		template,              // Service certificate template
		caCert,                // Parent CA certificate
		&privateKey.PublicKey, // Public key of service
		caKey,                 // Private key of CA (used for signing)
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create service certificate: %w", err)
	}

	// Encode to PEM
	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certDER,
	})
	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	})
	caCertPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: caCert.Raw,
	})

	// Create a Kubernetes Secret with the service cert
	// NOTE: what's the point of creating a kubernetes secret? And why are you creating a secret before
	// checking if a secret already exists?
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      config.SecretName,
			Namespace: config.Namespace,
		},
		Type: corev1.SecretTypeTLS,
		Data: map[string][]byte{
			corev1.TLSCertKey:       certPEM,   // "tls.crt" - service certificate
			corev1.TLSPrivateKeyKey: keyPEM,    // "tls.key" - service private key
			"ca.crt":                caCertPEM, // CA cert for verification
		},
	}

	// Check if secret already exists
	existingSecret := &corev1.Secret{}
	err = m.client.Get(ctx, types.NamespacedName{
		Name:      config.SecretName,
		Namespace: config.Namespace,
	}, existingSecret)

	if err == nil {
		// Secret exists - update it
		existingSecret.Data = secret.Data
		if err = m.client.Update(ctx, existingSecret); err != nil {
			return nil, fmt.Errorf("failed to update service cert secret: %w", err)
		}
		return existingSecret, nil
	}

	if !errors.IsNotFound(err) {
		return nil, fmt.Errorf("failed to check for existing service cert secret: %w", err)
	}

	// Secret doesn't exist - create it
	if err := m.client.Create(ctx, secret); err != nil {
		return nil, fmt.Errorf("failed to create service cert secret: %w", err)
	}

	return secret, nil
}

// GetServiceCertSecretName returns the secret name for a component's service certificate
// NOTE: do we need this? Feel like this should be delegated to the components, unless there's some shared function
func GetServiceCertSecretName(componentName string) string {
	return fmt.Sprintf("%s-service-cert", componentName)
}
