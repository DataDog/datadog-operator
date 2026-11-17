// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package resolver

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	secrets "github.com/DataDog/datadog-agent/comp/core/secrets/def"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	dummyCert = "-----BEGIN CERTIFICATE-----\nDUMMYCERT\n-----END CERTIFICATE-----\n"
	dummyKey  = "-----BEGIN PRIVATE KEY-----\nDUMMYKEY\n-----END PRIVATE KEY-----\n"
)

// backendOutput is returned for every call; the resolver ignores handles it didn't ask for.
const backendOutput = `{
"ns/client;tls.crt": {"value": "-----BEGIN CERTIFICATE-----\nDUMMYCERT\n-----END CERTIFICATE-----\n"},
"ns/client;tls.key": {"value": "-----BEGIN PRIVATE KEY-----\nDUMMYKEY\n-----END PRIVATE KEY-----\n"},
"prod/password": {"value": "DUMMYPASSWORD"},
"ns/other;y": {"value": "OTHERSECRET"},
"ns/looks;enc": {"value": "ENC[ns/other;y]"},
"ns/bad;x": {"error": "backend said DUMMYBACKENDERROR"}
}`

// newTestResolver returns a Resolver using a fake backend, and its call count.
func newTestResolver(t *testing.T) (*Resolver, func() int) {
	t.Helper()
	return newTestResolverWithParams(t, secrets.ConfigParams{})
}

func newTestResolverWithParams(t *testing.T, params secrets.ConfigParams) (*Resolver, func() int) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake backend is a shell script")
	}
	dir := t.TempDir()
	calls := filepath.Join(dir, "calls")
	script := filepath.Join(dir, "backend.sh")
	body := fmt.Sprintf("#!/bin/sh\ncat > /dev/null\necho x >> %q\ncat <<'EOF'\n%s\nEOF\n", calls, backendOutput)
	require.NoError(t, os.WriteFile(script, []byte(body), 0o700))

	params.Command = script
	r := New(params, nil)
	return r, func() int {
		b, err := os.ReadFile(calls)
		if os.IsNotExist(err) {
			return 0
		}
		require.NoError(t, err)
		return strings.Count(string(b), "x")
	}
}

func TestResolveJSONWithoutReferences(t *testing.T) {
	for name, in := range map[string]string{
		"plain":       `{"name": "test",   "n": 1.0, "big": 12345678901234567890}`,
		"inline":      `{"message": "token is ENC[prod/password] here"}`,
		"key only":    `{"ENC[prod/password]": "value"}`,
		"invalid":     `{"cert": "ENC[ns/client;tls.crt]"`,
		"trailing":    `{"cert": "ENC[ns/client;tls.crt]"} {}`,
		"not encoded": `{"cert": "ENC(ns/client;tls.crt)"}`,
	} {
		t.Run(name, func(t *testing.T) {
			r, calls := newTestResolver(t)
			res, err := r.ResolveJSON(in, "origin", "ns")
			require.NoError(t, err)
			assert.Equal(t, Result{JSON: in}, res, "input must be returned byte for byte")
			assert.Zero(t, calls())
		})
	}
}

func TestResolveJSON(t *testing.T) {
	r, calls := newTestResolver(t)
	in := `{
		"name": "api test",
		"n": 1, "f": 1.5, "big": 12345678901234567890, "b": true, "z": null, "e": {}, "a": [],
		"yes": "yes", "octal": "0755", "date": "2024-01-01", "dotted.key": "v", "uni": "héllo ✓",
		"esc": "x\/y", "huge": 123456789012345678901234567890, "<<": {"m": 1}, "html": "<a & b>",
		"certificate": {
			"cert": {"content": "ENC[ns/client;tls.crt]"},
			"key": {"content": "  ENC[ns/client;tls.key]\t"}
		},
		"list": ["ENC[prod/password]", 2, {"p": "ENC[prod/password]"}]
	}`
	want := fmt.Sprintf(`{
		"name": "api test",
		"n": 1, "f": 1.5, "big": 12345678901234567890, "b": true, "z": null, "e": {}, "a": [],
		"yes": "yes", "octal": "0755", "date": "2024-01-01", "dotted.key": "v", "uni": "héllo ✓",
		"esc": "x/y", "huge": 123456789012345678901234567890, "<<": {"m": 1}, "html": "<a & b>",
		"certificate": {
			"cert": {"content": %q},
			"key": {"content": %q}
		},
		"list": ["DUMMYPASSWORD", 2, {"p": "DUMMYPASSWORD"}]
	}`, dummyCert, dummyKey)

	res, err := r.ResolveJSON(in, "origin", "ns")
	require.NoError(t, err)
	out := res.JSON
	assert.JSONEq(t, want, out)
	assert.ElementsMatch(t, []string{dummyCert, dummyKey, "DUMMYPASSWORD"}, res.Values)
	assert.False(t, res.NoBackend)
	assert.Contains(t, out, `"big":12345678901234567890`, "large integers must keep their digits")
	assert.Contains(t, out, `"huge":123456789012345678901234567890`)
	assert.Contains(t, out, `"html":"<a & b>"`, "no HTML escaping")
	assert.Equal(t, 3, calls())

	// Cached handles don't run the backend again.
	_, err = r.ResolveJSON(in, "origin", "ns")
	require.NoError(t, err)
	assert.Equal(t, 3, calls())
}

func TestResolveJSONTopLevelArray(t *testing.T) {
	r, _ := newTestResolver(t)
	res, err := r.ResolveJSON(`["ENC[prod/password]", 1]`, "origin", "ns")
	require.NoError(t, err)
	assert.JSONEq(t, `["DUMMYPASSWORD", 1]`, res.JSON)
}

func TestResolveJSONValueLooksLikeReference(t *testing.T) {
	r, _ := newTestResolver(t)
	res, err := r.ResolveJSON(`{"v": "ENC[ns/looks;enc]"}`, "origin", "ns")
	require.NoError(t, err)
	assert.JSONEq(t, `{"v": "ENC[ns/other;y]"}`, res.JSON)

	// The cached value must stay literal even when its apparent reference is new.
	res, err = r.ResolveJSON(`{"v": "ENC[ns/looks;enc]", "other": "ENC[ns/other;y]"}`, "origin", "ns")
	require.NoError(t, err)
	assert.JSONEq(t, `{"v": "ENC[ns/other;y]", "other": "OTHERSECRET"}`, res.JSON)
}

func TestResolveJSONUnresolved(t *testing.T) {
	for name, tc := range map[string]struct {
		in       string
		handles  []string
		resolved []string
	}{
		"backend error": {
			in:      `{"v": "ENC[ns/bad;x]"}`,
			handles: []string{"ns/bad;x"},
		},
		"missing handle": {
			in:      `{"v": "ENC[ns/missing;x]"}`,
			handles: []string{"ns/missing;x"},
		},
		"partially resolved": {
			in:       `{"ok": "ENC[ns/client;tls.key]", "bad": "ENC[ns/bad;x]", "p": "ENC[prod/password]"}`,
			handles:  []string{"ns/bad;x"},
			resolved: []string{"ns/client;tls.key", "prod/password"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			r, _ := newTestResolver(t)
			res, err := r.ResolveJSON(tc.in, "origin", "ns")
			require.ErrorIs(t, err, ErrUnresolved)
			assert.Equal(t, Result{}, res, "a partially resolved payload must not be returned")
			for _, h := range tc.handles {
				assert.Contains(t, err.Error(), h)
			}
			for _, h := range tc.resolved {
				assert.NotContains(t, err.Error(), h)
			}
			for _, s := range []string{"DUMMYKEY", "DUMMYPASSWORD", "DUMMYBACKENDERROR"} {
				assert.NotContains(t, err.Error(), s)
			}
		})
	}
}

func TestResolveJSONNoBackend(t *testing.T) {
	r := New(secrets.ConfigParams{}, nil)
	in := `{"v": "ENC[prod/password]"}`
	res, err := r.ResolveJSON(in, "origin", "ns")
	require.NoError(t, err)
	assert.Equal(t, Result{JSON: in, NoBackend: true}, res, "references are left as is")

	in = `{"v": "plain"}`
	res, err = r.ResolveJSON(in, "origin", "ns")
	require.NoError(t, err)
	assert.Equal(t, Result{JSON: in}, res)
}

func TestResolveJSONInvalidConfig(t *testing.T) {
	r := New(secrets.ConfigParams{Type: "k8s.secrets"}, fmt.Errorf("config contains SENSITIVE"))
	res, err := r.ResolveJSON(`{"v": "ENC[a]"}`, "origin", "ns")
	require.ErrorIs(t, err, ErrUnresolved)
	assert.NotContains(t, err.Error(), "SENSITIVE")
	assert.Equal(t, Result{}, res)

	res, err = r.ResolveJSON(`{"v": "plain"}`, "origin", "ns")
	require.NoError(t, err)
	assert.Equal(t, Result{JSON: `{"v": "plain"}`}, res)
}

func TestResolveJSONDefaultsAndCommandPrecedence(t *testing.T) {
	// The explicit command ignores connector config, including unserializable values.
	r, _ := newTestResolverWithParams(t, secrets.ConfigParams{
		Type:   "k8s.secrets",
		Config: map[string]any{"ignored": make(chan int)},
	})
	res, err := r.ResolveJSON(`{"v": "ENC[prod/password]"}`, "origin", "ns")
	require.NoError(t, err)
	assert.JSONEq(t, `{"v": "DUMMYPASSWORD"}`, res.JSON)
}
