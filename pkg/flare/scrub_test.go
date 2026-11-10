// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flare

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
)

const (
	testAPIKey = "3290abeefc68e1bbe852a25252bad88c"
	testAppKey = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaabbbb"
	testToken  = "s3cr3t-T0ken-Value"
	testPass   = "p@ssw0r)"
)

const testCert = `-----BEGIN PRIVATE KEY-----
MIICdQIBADANBgkqhkiG9w0BAQEFAASCAl8wggJbAgEAAoGBAOLJKRals8tGoy7K
ljG6/hMcoe16W6MPn47Q601ttoFkMoSJZ1Jos6nxn32KXfG6hCiB0bmf1iyZtaMa
-----END PRIVATE KEY-----`

// TestScrubBytes covers the cases of the scrubber the kubectl plugin used
// before, and the camelCase, JSON and environment forms the Operator writes.
func TestScrubBytes(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		secrets []string
		keep    []string
	}{
		{name: "yaml api key", input: "api_key: " + testAPIKey, secrets: []string{testAPIKey}},
		{name: "quoted api key", input: `api_key: "` + testAPIKey + `"`, secrets: []string{testAPIKey}},
		{name: "api key in a block", input: "api_key: |\n  " + testAPIKey, secrets: []string{testAPIKey}},
		{name: "additional endpoints", input: "additional_endpoints:\n  \"https://app.datadoghq.com\":\n  - " + testAPIKey + ",", secrets: []string{testAPIKey}},
		{name: "app key", input: "app_key: '" + testAppKey + "'", secrets: []string{testAppKey}},
		{name: "api key in a url", input: "Error status code 500 : http://dog.tld/api?key=" + testAPIKey, secrets: []string{testAPIKey}},
		{name: "url password", input: "random_url_key: http://user:password@host:port", secrets: []string{"password"}, keep: []string{"host:port"}},
		{name: "ftp password in text", input: "Connection dropped : ftp://user:" + testPass + "@host:port", secrets: []string{testPass}},
		{name: "yaml password", input: "mysql_password: " + testPass, secrets: []string{testPass}},
		{name: "pwd", input: "pwd: 'password'", secrets: []string{"'password'"}},
		{name: "yaml token", input: "token: " + testToken, secrets: []string{testToken}},
		{name: "camelCase token", input: "    clusterAgentToken: " + testToken, secrets: []string{testToken}},
		{name: "camelCase api key", input: "  apiKey: " + testToken, secrets: []string{testToken}},
		{name: "json token", input: `{"level":"info","authToken":"` + testToken + `","msg":"ok"}`, secrets: []string{testToken}, keep: []string{`"msg":"ok"`}},
		{name: "json api key", input: `{"apiKey": "` + testToken + `"}`, secrets: []string{testToken}},
		{name: "env api key", input: `"DD_API_KEY=` + testToken + `",`, secrets: []string{testToken}},
		{name: "flag token", input: "--cluster-agent-token=" + testToken, secrets: []string{testToken}},
		{name: "bearer token", input: "Authorization: Bearer " + testToken, secrets: []string{testToken}},
		{name: "certificate", input: "cert_key: |\n" + testCert, secrets: []string{"MIICdQIBADANBgkqhkiG9w0BAQEFAASCAl8wggJbAgEAAoGBAOLJKRals8tGoy7K"}},
		{name: "container id is kept", input: `container_id: "b32bd6f9b73ba7ccb64953a04b82b48e29dfafab65fd57ca01d3b94a0e024885"`, keep: []string{"b32bd6f9b73ba7ccb64953a04b82b48e29dfafab65fd57ca01d3b94a0e024885"}},
		{name: "booleans are kept", input: "automountServiceAccountToken: true", keep: []string{"automountServiceAccountToken: true"}},
		{name: "secret references are kept", input: "secretName: datadog-secret\nkeyName: api-key", keep: []string{"secretName: datadog-secret", "keyName: api-key"}},
		{name: "comments are kept", input: "# HELP controller_runtime_reconcile_total Total reconciliations\nfoo 1", keep: []string{"# HELP controller_runtime_reconcile_total"}},
	}
	s := NewScrubber()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := string(s.ScrubBytes([]byte(tt.input)))
			for _, secret := range tt.secrets {
				assert.NotContains(t, out, secret)
			}
			for _, kept := range tt.keep {
				assert.Contains(t, out, kept)
			}
		})
	}
}

func TestScrubBytesLongLine(t *testing.T) {
	// The agent scrubber's line scanner fails on lines over 64KiB.
	line := strings.Repeat("x", 200*1024) + " api_key: " + testAPIKey
	out := string(NewScrubber().ScrubBytes([]byte(line + "\nnext line")))
	assert.NotContains(t, out, testAPIKey)
	assert.Contains(t, out, "next line")
}

func TestRedactObject(t *testing.T) {
	obj := map[string]any{
		"spec": map[string]any{
			"global": map[string]any{
				"credentials": map[string]any{
					"apiKey": testAPIKey,
					"appKey": testAppKey,
					"apiSecret": map[string]any{
						"secretName": "datadog-secret",
						"keyName":    "api-key",
					},
				},
				"clusterAgentToken": testToken,
			},
		},
		"webhook": map[string]any{"caBundle": "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0t"},
	}
	container := corev1.Container{Env: []corev1.EnvVar{
		{Name: "DD_API_KEY", Value: testAPIKey},
		{Name: "DD_SITE", Value: "datadoghq.com"},
		{Name: "DD_APP_KEY", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{Key: "app-key", LocalObjectReference: corev1.LocalObjectReference{Name: "datadog-secret"}}}},
	}}

	a := NewArchive(ArchiveOptions{})
	require.NoError(t, a.AddYAML("dda.yaml", obj))
	require.NoError(t, a.AddJSON("container.json", container))
	data, err := a.Close(nil)
	require.NoError(t, err)
	files := readZip(t, data)

	for _, content := range files {
		for _, secret := range []string{testAPIKey, testAppKey, testToken, "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0t"} {
			assert.NotContains(t, content, secret)
		}
	}
	assert.Contains(t, files["dda.yaml"], "secretName: datadog-secret")
	assert.Contains(t, files["container.json"], "datadoghq.com")
	assert.Contains(t, files["container.json"], "datadog-secret")
}

func TestIsSensitiveKey(t *testing.T) {
	for _, key := range []string{"DD_API_KEY", "apiKey", "app_key", "clusterAgentToken", "password", "caBundle", "DD_SECRET", "privateKey"} {
		assert.True(t, IsSensitiveKey(key), key)
	}
	for _, key := range []string{"secretName", "keyName", "key", "name", "DD_SITE", "tokenReviewEnabled"} {
		assert.False(t, IsSensitiveKey(key), key)
	}
}
