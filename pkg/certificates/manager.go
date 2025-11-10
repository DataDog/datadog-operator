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
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Manager handles certificate operations
type Manager struct {
	client client.Client
}

// NewManager creates a new certificate manager
func NewManager(client client.Client) *Manager {
	return &Manager{
		client: client,
	}
}

const (
	// CASecretName is the name of the Kubernetes Secret containing the CA certificate and private key
	CASecretName = "datadog-agent-cluster-ca-secret"

	// CAValidityYears is the validity period for the CA certificate
	CAValidityYears = 50
)

// GetOrCreateCA retrieves an existing CA certificate or creates a new one if it doesn't exist.
// The CA certificate and private key are stored in a Kubernetes Secret.
// Returns the CA certificate and private key.
func (m *Manager) GetOrCreateCA(ctx context.Context, namespace string) error {
	// Try to get existing CA from Secret
	secret := &corev1.Secret{}
	secretKey := client.ObjectKey{
		Namespace: namespace,
		Name:      CASecretName,
	}

	err := m.client.Get(ctx, secretKey, secret)
	if err == nil {
		return nil
	}

	if !errors.IsNotFound(err) {
		return fmt.Errorf("failed to get CA secret: %w", err)
	}

	// CA doesn't exist, generate new one
	caCert, caKey, err := m.generateCA()
	if err != nil {
		return fmt.Errorf("failed to generate CA: %w", err)
	}

	// Store in the same namespace as DatadogAgent
	if err := m.storeCA(ctx, namespace, caCert, caKey); err != nil {
		return fmt.Errorf("failed to store CA: %w", err)
	}

	return nil
}

// generateCA generates a new self-signed CA certificate with a 50-year validity period
func (m *Manager) generateCA() (*x509.Certificate, *rsa.PrivateKey, error) {
	// Generate RSA private key (4096-bit for CA)
	caKey, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate CA private key: %w", err)
	}

	// Create CA certificate template
	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate serial number: %w", err)
	}

	notBefore := time.Now()
	notAfter := notBefore.Add(time.Duration(CAValidityYears) * 365 * 24 * time.Hour)

	caTemplate := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName:   "Datadog Agent CA",
			Organization: []string{"Datadog"},
		},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            0,
		MaxPathLenZero:        true,
	}

	// Self-sign the CA certificate
	caCertDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create CA certificate: %w", err)
	}

	// Parse the DER-encoded certificate
	caCert, err := x509.ParseCertificate(caCertDER)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse CA certificate: %w", err)
	}

	return caCert, caKey, nil
}

// storeCA stores the CA certificate and private key in a Kubernetes Secret
func (m *Manager) storeCA(ctx context.Context, namespace string, caCert *x509.Certificate, caKey *rsa.PrivateKey) error {
	// Encode certificate to PEM
	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: caCert.Raw,
	})

	// Encode private key to PEM (PKCS1 format)
	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(caKey),
	})

	// Create Secret
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      CASecretName,
			Namespace: namespace,
		},
		Type: corev1.SecretTypeTLS,
		Data: map[string][]byte{
			"tls.crt": certPEM,
			"tls.key": keyPEM,
		},
	}

	// Create or update the Secret
	err := m.client.Create(ctx, secret)
	if err != nil {
		if errors.IsAlreadyExists(err) {
			// Update existing secret
			return m.client.Update(ctx, secret)
		}
		return fmt.Errorf("failed to create CA secret: %w", err)
	}

	return nil
}
