// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package secrets

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
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

	payload := map[string]interface{}{
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
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("error while running '%s': command timeout", sb.cmd)
		}
		return nil, fmt.Errorf("error while running '%s': %s", sb.cmd, err)
	}

	return stdout.buf.Bytes(), nil
}

// isConfigured returns true if the secret backend command is configured
func (sb *SecretBackend) isConfigured() bool {
	return sb.cmd != ""
}
