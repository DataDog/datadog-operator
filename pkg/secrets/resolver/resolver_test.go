// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package resolver

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-operator/pkg/secrets"
)

const (
	dummyCert = "-----BEGIN CERTIFICATE-----\nDUMMYCERT\n-----END CERTIFICATE-----\n"
	dummyKey  = "-----BEGIN PRIVATE KEY-----\nDUMMYKEY\n-----END PRIVATE KEY-----\n"
)

var dummyValues = map[string]string{
	"ns/client;tls.crt": dummyCert,
	"ns/client;tls.key": dummyKey,
	"prod/password":     "DUMMYPASSWORD",
	"ns/other;y":        "OTHERSECRET",
	"ns/looks;enc":      "ENC[ns/other;y]",
}

// fakeBackend resolves dummyValues, fails other handles and counts calls.
type fakeBackend struct{ calls int }

func (f *fakeBackend) Decrypt(refs []string) (map[string]string, error) {
	f.calls++
	out := map[string]string{}
	for _, ref := range refs {
		h, _ := secrets.ParseEnc(ref)
		v, ok := dummyValues[h]
		if !ok {
			return nil, errors.New("backend said DUMMYBACKENDERROR")
		}
		out[ref] = v
	}
	return out, nil
}

func newTestResolver() (*Resolver, *fakeBackend) {
	b := &fakeBackend{}
	return New(b, true, nil), b
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
			r, b := newTestResolver()
			res, err := r.ResolveJSON(in)
			require.NoError(t, err)
			assert.Equal(t, Result{JSON: in}, res, "input must be returned byte for byte")
			assert.Zero(t, b.calls)
		})
	}
}

func TestResolveJSON(t *testing.T) {
	r, b := newTestResolver()
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

	res, err := r.ResolveJSON(in)
	require.NoError(t, err)
	out := res.JSON
	assert.JSONEq(t, want, out)
	assert.ElementsMatch(t, []string{dummyCert, dummyKey, "DUMMYPASSWORD"}, res.Values)
	assert.False(t, res.NoBackend)
	assert.Contains(t, out, `"big":12345678901234567890`, "large integers must keep their digits")
	assert.Contains(t, out, `"huge":123456789012345678901234567890`)
	assert.Contains(t, out, `"html":"<a & b>"`, "no HTML escaping")
	assert.Equal(t, 3, b.calls, "one backend call per unique handle")
}

func TestResolveJSONTopLevelArray(t *testing.T) {
	r, _ := newTestResolver()
	res, err := r.ResolveJSON(`["ENC[prod/password]", 1]`)
	require.NoError(t, err)
	assert.JSONEq(t, `["DUMMYPASSWORD", 1]`, res.JSON)
}

func TestResolveJSONEscapedReference(t *testing.T) {
	in := `{"p": "ENC[prod/password]"}`
	assert.True(t, HasReferences(in))
	r, _ := newTestResolver()
	res, err := r.ResolveJSON(in)
	require.NoError(t, err)
	assert.JSONEq(t, `{"p": "DUMMYPASSWORD"}`, res.JSON)
}

func TestResolveJSONValueLooksLikeReference(t *testing.T) {
	r, _ := newTestResolver()
	res, err := r.ResolveJSON(`{"v": "ENC[ns/looks;enc]", "other": "ENC[ns/other;y]"}`)
	require.NoError(t, err)
	assert.JSONEq(t, `{"v": "ENC[ns/other;y]", "other": "OTHERSECRET"}`, res.JSON, "resolved values stay literal")
}

func TestResolveJSONUnresolved(t *testing.T) {
	for name, tc := range map[string]struct {
		in       string
		handles  []string
		resolved []string
	}{
		"missing handle": {
			in:      `{"v": "ENC[ns/missing;x]"}`,
			handles: []string{"ns/missing;x"},
		},
		"partially resolved": {
			in:       `{"ok": "ENC[ns/client;tls.key]", "bad": "ENC[ns/bad;x]", "p": "ENC[prod/password]"}`,
			handles:  []string{"ns/bad;x"},
			resolved: []string{"ns/client;tls.key", "prod/password"},
		},
		"empty reference": {
			in:      `{"v": " ENC[] ", "p": "ENC[prod/password]"}`,
			handles: []string{"empty ENC[] reference"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			r, _ := newTestResolver()
			res, err := r.ResolveJSON(tc.in)
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
	b := &fakeBackend{}
	r := New(b, false, nil)
	in := `{"v": "ENC[prod/password]"}`
	res, err := r.ResolveJSON(in)
	require.NoError(t, err)
	assert.Equal(t, Result{JSON: in, NoBackend: true}, res, "references are left as is")

	in = `{"v": "plain"}`
	res, err = r.ResolveJSON(in)
	require.NoError(t, err)
	assert.Equal(t, Result{JSON: in}, res)
	assert.Zero(t, b.calls)
}

func TestResolveJSONInvalidConfig(t *testing.T) {
	b := &fakeBackend{}
	r := New(b, true, fmt.Errorf("config contains SENSITIVE"))
	res, err := r.ResolveJSON(`{"v": "ENC[prod/password]"}`)
	require.ErrorIs(t, err, ErrUnresolved)
	assert.NotContains(t, err.Error(), "SENSITIVE")
	assert.Equal(t, Result{}, res)

	res, err = r.ResolveJSON(`{"v": "plain"}`)
	require.NoError(t, err)
	assert.Equal(t, Result{JSON: `{"v": "plain"}`}, res)
	assert.Zero(t, b.calls)
}

func TestResolveJSONWithSecretBackend(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake backend is a shell script")
	}
	script := filepath.Join(t.TempDir(), "backend.sh")
	body := `#!/bin/sh
cat > /dev/null
cat <<'EOF'
{"ns/client;tls.key": {"value": "-----BEGIN PRIVATE KEY-----\nDUMMYKEY\n-----END PRIVATE KEY-----\n"}, "ns/bad;x": {"error": "DUMMYBACKENDERROR"}}
EOF
`
	require.NoError(t, os.WriteFile(script, []byte(body), 0o700))
	secrets.SetSecretBackendCommand(script)
	t.Cleanup(func() { secrets.SetSecretBackendCommand("") })

	r := New(secrets.NewSecretBackend(), true, nil)
	res, err := r.ResolveJSON(`{"key": "ENC[ns/client;tls.key]"}`)
	require.NoError(t, err)
	assert.JSONEq(t, fmt.Sprintf(`{"key": %q}`, dummyKey), res.JSON)

	_, err = r.ResolveJSON(`{"key": "ENC[ns/client;tls.key]", "bad": "ENC[ns/bad;x]"}`)
	require.ErrorIs(t, err, ErrUnresolved)
	assert.Contains(t, err.Error(), "ns/bad;x")
	assert.NotContains(t, err.Error(), "DUMMY")
}
