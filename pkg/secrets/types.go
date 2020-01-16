// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package secrets

import "time"

// Decryptor is used to decrypt encrypted secrets
// Decryptor is implemented by SecretBackend
type Decryptor interface {
	Decrypt([]string) (map[string]string, error)
}

// SecretBackend retrieves secrets from secret backend binary
// SecretBackend implements the Decryptor interface
type SecretBackend struct {
	cmd              string
	cmdOutputMaxSize int
	cmdTimeout       time.Duration
}

// secret defines the structure for secrets in JSON output
type secret struct {
	Value    string `json:"value,omitempty"`
	ErrorMsg string `json:"error,omitempty"`
}
