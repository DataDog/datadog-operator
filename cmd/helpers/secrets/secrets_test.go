// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package secrets

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	s "github.com/DataDog/datadog-operator/pkg/secrets"
)

func TestReadSecretsDirectoryBoundary(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "secrets")
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "..2026"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "..2026", "key"), []byte("inside\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(base, "outside"), []byte("OUTSIDESECRET"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "large"), []byte(strings.Repeat("x", maxSecretFileSize+1)), 0o600))
	for name, target := range map[string]string{
		"..data":    "..2026",
		"key":       "..data/key",
		"escape":    "../outside",
		"chain":     "escape",
		"directory": "..",
		"absolute":  filepath.Join(base, "outside"),
	} {
		require.NoError(t, os.Symlink(target, filepath.Join(dir, name)))
	}

	for _, tc := range []struct {
		name string
		want string
	}{
		{name: "key", want: "inside\n"},
		{name: "..data/key", want: "inside\n"},
		{name: "../outside"},
		{name: filepath.Join(base, "outside")},
		{name: "escape"},
		{name: "chain"},
		{name: "directory/outside"},
		{name: "absolute"},
		{name: "large"},
		{name: "missing"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request, err := json.Marshal(secretsRequest{Version: s.PayloadVersion, Secrets: []string{tc.name}})
			require.NoError(t, err)
			var out bytes.Buffer
			require.NoError(t, readSecrets(bytes.NewReader(request), &out, dir))
			var response map[string]s.Secret
			require.NoError(t, json.Unmarshal(out.Bytes(), &response))
			secret := response[tc.name]
			assert.Equal(t, tc.want, secret.Value)
			if tc.want == "" {
				assert.NotEmpty(t, secret.ErrorMsg)
			} else {
				assert.Empty(t, secret.ErrorMsg)
			}
			assert.NotContains(t, out.String(), "OUTSIDESECRET")
		})
	}
}
