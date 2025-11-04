// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package certificates

const (
	// Name of Certificate Authority secret
	CASecretName = "datadog-agent-cluster-ca"
	// CAValidityYears is the validity period for the CA certificate
	CAValidityYears = 50
	// ServiceValidateDays is the validity period for service certificates (1 year)
	ServiceCertValidityDays = 365
)

// ServiceCertConfig contains parameters for generating a service certificate
type ServiceCertConfig struct {
	// SecretName is the name of the Kubernetes secret to create
	SecretName string

	// CommonName is the primary identity of the certificate (e.g., "datadog-cluster-agent")
	// NOTE: come back to this... what's the point
	CommonName string

	//DNSNames are all DNS names this service can be reached at
	// Example: ["datadog-cluster-agent", "datadog-cluster-agent.default.svc.cluster.local"]
	DNSNames []string

	// Organization is the organization name for the certificate
	Organizations []string

	// Namespace is the Kubernetes namesapce where this service runs
	// NOTE: come back to this, is it needed?
	Namespace string
}
