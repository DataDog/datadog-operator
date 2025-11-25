// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package secrets

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/pkg/constants"
)

var (
	secretBackendCommand = ""
	secretBackendArgs    = []string{}
)

const (
	defaultCmdOutputMaxSize = 1024 * 1024
	defaultCmdTimeout       = 5 * time.Second

	// PayloadVersion represents the version of the SB API
	PayloadVersion = "1.0"
)

// SetSecretBackendCommand set the secretBackendCommand var
func SetSecretBackendCommand(command string) {
	secretBackendCommand = command
}

// SetSecretBackendArgs set the secretBackendArgs var
func SetSecretBackendArgs(args []string) {
	secretBackendArgs = args
}

// NewSecretBackend returns a new SecretBackend instance
func NewSecretBackend() *SecretBackend {
	return &SecretBackend{
		cmd:              secretBackendCommand,
		cmdArgs:          secretBackendArgs,
		cmdOutputMaxSize: defaultCmdOutputMaxSize,
		cmdTimeout:       defaultCmdTimeout,
	}
}

// Decrypt tries to decrypt a given string slice using the secret backend command
func (sb *SecretBackend) Decrypt(encrypted []string) (map[string]string, error) {
	if !sb.isConfigured() {
		return nil, NewDecryptorError(errors.New("secret backend command not configured"), false)
	}

	return sb.fetchSecret(encrypted)
}

// fetchSecret tries to get secrets by executing the secret backend command
func (sb *SecretBackend) fetchSecret(encrypted []string) (map[string]string, error) {
	handles, err := extractHandles(encrypted)
	if err != nil {
		return nil, NewDecryptorError(err, false)
	}

	payload := map[string]any{
		"version": PayloadVersion,
		"secrets": handles,
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return nil, NewDecryptorError(err, false)
	}

	output, err := sb.execCommand(string(jsonPayload))
	if err != nil {
		return nil, NewDecryptorError(err, true)
	}

	secrets := map[string]Secret{}
	err = json.Unmarshal(output, &secrets)
	if err != nil {
		return nil, NewDecryptorError(err, true)
	}

	decrypted := map[string]string{}
	for _, handle := range handles {
		secretHandle, found := secrets[handle]
		if !found {
			return nil, NewDecryptorError(fmt.Errorf("secret handle '%s' was not decrypted by the secret_backend_command", handle), false)
		}
		if secretHandle.ErrorMsg != "" {
			return nil, NewDecryptorError(fmt.Errorf("an error occurred while decrypting '%s': %s", handle, secretHandle.ErrorMsg), false)
		}
		if secretHandle.Value == "" {
			return nil, NewDecryptorError(fmt.Errorf("decrypted secret for '%s' is empty", handle), false)
		}

		decrypted[encFormat(handle)] = secretHandle.Value
	}

	return decrypted, nil
}

// execCommand executes the secret backend command
func (sb *SecretBackend) execCommand(inputPayload string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), sb.cmdTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, sb.cmd, sb.cmdArgs...)

	cmd.Stdin = strings.NewReader(inputPayload)

	stdout := limitBuffer{
		buf: &bytes.Buffer{},
		max: sb.cmdOutputMaxSize,
	}
	stderr := limitBuffer{
		buf: &bytes.Buffer{},
		max: sb.cmdOutputMaxSize,
	}
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, fmt.Errorf("error while running '%s': command timeout", sb.cmd)
		}
		return nil, fmt.Errorf("error while running '%s': %w", sb.cmd, err)
	}

	return stdout.buf.Bytes(), nil
}

// isConfigured returns true if the secret backend command is configured
func (sb *SecretBackend) isConfigured() bool {
	return sb.cmd != ""
}

// GetDefaultCredentialsSecretName returns the default name for credentials secret
func GetDefaultCredentialsSecretName(dda metav1.Object) string {
	return dda.GetName() + "-secret"
}

// GetDefaultDCATokenSecretName returns the default name for cluster-agent secret
func GetDefaultDCATokenSecretName(dda metav1.Object) string {
	return constants.GetDDAName(dda) + "-token"
}

// GetAPIKeySecret returns the API key secret name and the key inside the secret
// returns <isSet>, secretName, secretKey
// Note that the default name can differ depending on where this is called
func GetAPIKeySecret(credentials *v2alpha1.DatadogCredentials, defaultName string) (bool, string, string) {
	isSet := false
	secretName := defaultName
	secretKey := v2alpha1.DefaultAPIKeyKey

	if credentials.APISecret != nil {
		isSet = true
		secretName = credentials.APISecret.SecretName
		if credentials.APISecret.KeyName != "" {
			secretKey = credentials.APISecret.KeyName
		}
	} else if credentials.APIKey != nil && *credentials.APIKey != "" {
		isSet = true
	}

	return isSet, secretName, secretKey
}

// GetAppKeySecret returns the APP key secret name and the key inside the secret
// returns <isSet>, secretName, secretKey
// Note that the default name can differ depending on where this is called
func GetAppKeySecret(credentials *v2alpha1.DatadogCredentials, defaultName string) (bool, string, string) {
	isSet := false
	secretName := defaultName
	secretKey := v2alpha1.DefaultAPPKeyKey

	if credentials.AppSecret != nil {
		isSet = true
		secretName = credentials.AppSecret.SecretName
		if credentials.AppSecret.KeyName != "" {
			secretKey = credentials.AppSecret.KeyName
		}
	} else if credentials.AppKey != nil && *credentials.AppKey != "" {
		isSet = true
	}

	return isSet, secretName, secretKey
}

// GetKeysFromCredentials returns any key data that need to be stored in a new secret
func GetKeysFromCredentials(credentials *v2alpha1.DatadogCredentials) map[string][]byte {
	data := make(map[string][]byte)
	// Create secret using DatadogAgent credentials if it exists
	if credentials.APIKey != nil && *credentials.APIKey != "" {
		data[v2alpha1.DefaultAPIKeyKey] = []byte(*credentials.APIKey)
	}
	if credentials.AppKey != nil && *credentials.AppKey != "" {
		data[v2alpha1.DefaultAPPKeyKey] = []byte(*credentials.AppKey)
	}

	return data
}

// CheckAPIKeySufficiency use to check for the API key if:
// 1. an existing secret is defined, or
// 2. the corresponding env var is defined (whether in ENC format or not)
// If either of these is true, the secret is not needed.
func CheckAPIKeySufficiency(creds *v2alpha1.DatadogCredentials, envVarName string) bool {
	return ((creds.APISecret != nil && creds.APISecret.SecretName != "") ||
		os.Getenv(envVarName) != "")
}

// CheckAppKeySufficiency use to check for the APP key if:
// 1. an existing secret is defined, or
// 2. the corresponding env var is defined (whether in ENC format or not)
// If either of these is true, the secret is not needed.
func CheckAppKeySufficiency(creds *v2alpha1.DatadogCredentials, envVarName string) bool {
	return ((creds.AppSecret != nil && creds.AppSecret.SecretName != "") ||
		os.Getenv(envVarName) != "")
}
