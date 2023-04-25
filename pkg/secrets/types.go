// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package secrets

import (
	"errors"
	"fmt"
	"time"

	"github.com/stretchr/testify/mock"
)

// DecryptorError describes the error returned by a Decryptor
type DecryptorError struct {
	err       error
	retriable bool
}

// Error implements the Error interface
func (e *DecryptorError) Error() string {
	return e.Unwrap().Error()
}

// Unwrap implements the Error interface
func (e *DecryptorError) Unwrap() error {
	return e.err
}

// IsRetriable returns wether the error is retriable
func (e *DecryptorError) IsRetriable() bool {
	return e.retriable
}

// NewDecryptorError returns a new DecryptorError
func NewDecryptorError(err error, retriable bool) *DecryptorError {
	return &DecryptorError{
		err:       err,
		retriable: retriable,
	}
}

// Retriable can be used to evaluate whether an error should be retried
func Retriable(err error) bool {
	var decryptorErr *DecryptorError
	if errors.As(err, &decryptorErr) {
		return decryptorErr.IsRetriable()
	}

	return false
}

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

// NewDummyDecryptor returns a dummy decryptor for tests
// maxRetries is the number of retries before returning a nil error
// If maxRetries < 0 Decrypt directly returns a permanent error
// If maxRetries == 0 Decrypt directly returns a nil error
// If maxRetries > 0 Decrypt returns a retriable error until it's called maxRetries-times then returns a nil error
func NewDummyDecryptor(maxRetries int) *DummyDecryptor {
	return &DummyDecryptor{
		maxRetries: maxRetries,
	}
}

// DummyDecryptor can be used in other packages to mock the secret backend
type DummyDecryptor struct {
	mock.Mock
	maxRetries int
	retryCount int
}

// Decrypt is used for testing
func (d *DummyDecryptor) Decrypt(secrets []string) (map[string]string, error) {
	d.Called(secrets)
	if d.maxRetries < 0 {
		return nil, NewDecryptorError(errors.New("permanent error"), false)
	}

	d.retryCount++
	if d.retryCount < d.maxRetries {
		return nil, NewDecryptorError(errors.New("retriable error"), true)
	}

	res := map[string]string{}
	for _, secret := range secrets {
		res[secret] = fmt.Sprintf("DEC[%s]", secret)
	}

	return res, nil
}
