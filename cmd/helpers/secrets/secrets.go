// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package secrets

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	s "github.com/DataDog/datadog-operator/pkg/secrets"
)

const (
	maxSecretFileSize = 8192
)

// NewCmd implements reading secrets from a directory/volume mount
func NewCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "read-secret",
		Short: "Read secret from a directory",
		Long:  ``,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return readSecrets(os.Stdin, os.Stdout, args[0])
		},
	}
}

type secretsRequest struct {
	Version string   `json:"version"`
	Secrets []string `json:"secrets"`
}

// ReadSecrets implements a secrets reader from a directory/mount
func readSecrets(r io.Reader, w io.Writer, dir string) error {
	in, err := io.ReadAll(r)
	if err != nil {
		return err
	}

	var request secretsRequest
	err = json.Unmarshal(in, &request)
	if err != nil {
		return errors.New("failed to unmarshal json input")
	}

	version := splitVersion(request.Version)
	compatVersion := splitVersion(s.PayloadVersion)
	if version[0] != compatVersion[0] {
		return fmt.Errorf("incompatible protocol version %q", request.Version)
	}

	if len(request.Secrets) == 0 {
		return errors.New("no secrets listed in input")
	}

	root, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer root.Close()

	response := map[string]s.Secret{}
	for _, name := range request.Secrets {
		response[name] = readSecret(root, name)
	}

	out, err := json.Marshal(response)
	if err != nil {
		return err
	}
	_, err = w.Write(out)
	return err
}

func splitVersion(ver string) []string {
	return strings.SplitN(ver, ".", 2)
}

func readSecret(root *os.Root, name string) s.Secret {
	value, err := readSecretFile(root, name)
	var errMsg string
	if err != nil {
		errMsg = err.Error()
	}
	return s.Secret{Value: value, ErrorMsg: errMsg}
}

// readSecretFile follows symlinks only within the configured directory.
func readSecretFile(root *os.Root, name string) (string, error) {
	file, err := root.Open(name)
	if err != nil {
		if os.IsNotExist(err) {
			return "", errors.New("secret does not exist")
		}
		return "", err
	}
	defer file.Close()

	value, err := io.ReadAll(io.LimitReader(file, maxSecretFileSize+1))
	if err != nil {
		return "", err
	}
	if len(value) > maxSecretFileSize {
		return "", errors.New("secret exceeds max allowed size")
	}
	return string(value), nil
}
