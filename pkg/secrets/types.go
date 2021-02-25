// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package secrets

import (
	"fmt"
	"time"

	"github.com/stretchr/testify/mock"
)

// Decryptor is used to decrypt encrypted secrets
// Decryptor is implemented by SecretBackend
type Decryptor interface {
	Decrypt([]string) (map[string]string, error)
}

// SecretBackend retrieves secrets from secret backend binary
// SecretBackend implements the Decryptor interface
type SecretBackend struct {
	cmd              string
	cmdArgs          []string
	cmdOutputMaxSize int
	cmdTimeout       time.Duration
}

// Secret defines the structure for secrets in JSON output
type Secret struct {
	Value    string `json:"value,omitempty"`
	ErrorMsg string `json:"error,omitempty"`
}

// DummyDecryptor can be used in other packages to mock the secret backend
type DummyDecryptor struct {
	mock.Mock
}

// Decrypt is used for testing
func (d *DummyDecryptor) Decrypt(secrets []string) (map[string]string, error) {
	d.Called(secrets)
	res := map[string]string{}
	for _, secret := range secrets {
		res[secret] = fmt.Sprintf("DEC[%s]", secret)
	}
	return res, nil
}
