// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package resolver resolves ENC[] references in JSON using the Operator secret backend.
package resolver

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/DataDog/datadog-operator/pkg/secrets"
)

// ErrUnresolved is returned when a payload references secrets that can't be resolved.
var ErrUnresolved = errors.New("unable to resolve secret references")

// Resolver resolves whole-string ENC[] references in JSON values.
type Resolver struct {
	backend    secrets.Decryptor
	configured bool
	configErr  error
}

// Result is the outcome of ResolveJSON.
type Result struct {
	// JSON is the resolved payload, or the input when nothing was resolved.
	JSON string
	// Values are the resolved secret values, for redaction.
	Values []string
	// NoBackend is set when references were left as is because no backend is configured.
	NoBackend bool
}

// New returns a Resolver using backend; configErr blocks references if a backend is configured.
func New(backend secrets.Decryptor, configured bool, configErr error) *Resolver {
	return &Resolver{backend: backend, configured: configured, configErr: configErr}
}

// HasReferences reports whether jsonSpec contains whole-string secret references.
func HasReferences(jsonSpec string) bool {
	tree, ok := decode(jsonSpec)
	return ok && len(findHandles(tree)) > 0
}

// ResolveJSON replaces references in jsonSpec with their values. Errors name handles, never values.
func (r *Resolver) ResolveJSON(jsonSpec string) (Result, error) {
	unchanged := Result{JSON: jsonSpec}
	// Invalid JSON is left for the caller to report.
	tree, ok := decode(jsonSpec)
	if !ok {
		return unchanged, nil
	}
	handles := findHandles(tree)
	if len(handles) == 0 {
		return unchanged, nil
	}
	if !r.configured {
		unchanged.NoBackend = true
		return unchanged, nil
	}
	if r.configErr != nil {
		return Result{}, fmt.Errorf("%w: invalid secret backend configuration", ErrUnresolved)
	}
	if slices.Contains(handles, "") {
		return Result{}, fmt.Errorf("%w: empty ENC[] reference", ErrUnresolved)
	}

	values, err := r.fetch(handles)
	if err != nil {
		return Result{}, err
	}
	out, err := encode(substitute(tree, values))
	if err != nil {
		return Result{}, ErrUnresolved
	}
	res := Result{JSON: out}
	for _, h := range handles {
		res.Values = append(res.Values, values[h])
	}
	return res, nil
}

// fetch resolves all handles in one backend run. If a handle fails, each is retried alone so the
// error names only the failing ones. Backend error text is dropped: it isn't guaranteed value-free.
func (r *Resolver) fetch(handles []string) (map[string]string, error) {
	values, err := r.decrypt(handles)
	if err == nil {
		return values, nil
	}
	if secrets.Retriable(err) {
		// The run itself failed (exec, timeout, output); retrying each handle would only repeat it.
		return nil, fmt.Errorf("%w: secret backend failed", ErrUnresolved)
	}
	var left []string
	for _, h := range handles {
		if _, err := r.decrypt([]string{h}); err != nil {
			left = append(left, h)
		}
	}
	if len(left) == 0 {
		// Each resolved alone but not together; can't tell which one is at fault.
		left = handles
	}
	return nil, fmt.Errorf("%w: %s", ErrUnresolved, strings.Join(left, ", "))
}

// decrypt resolves handles in one backend run, keyed by handle.
func (r *Resolver) decrypt(handles []string) (map[string]string, error) {
	refs := make([]string, len(handles))
	for i, h := range handles {
		refs[i] = "ENC[" + h + "]"
	}
	out, err := r.backend.Decrypt(refs)
	if err != nil {
		return nil, err
	}
	values := make(map[string]string, len(handles))
	for i, h := range handles {
		v, ok := out[refs[i]]
		if !ok {
			return nil, errors.New("missing value")
		}
		values[h] = v
	}
	return values, nil
}

// decode parses a single JSON value, keeping numbers as written.
func decode(s string) (any, bool) {
	dec := json.NewDecoder(strings.NewReader(s))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, false
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, false
	}
	return v, true
}

func encode(v any) (string, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return "", err
	}
	return strings.TrimSuffix(buf.String(), "\n"), nil
}

// findHandles returns the sorted, unique handles in string values (not map keys).
func findHandles(v any) []string {
	var handles []string
	walk(v, func(s string) string {
		if h, ok := secrets.ParseEnc(s); ok && !slices.Contains(handles, h) {
			handles = append(handles, h)
		}
		return s
	})
	slices.Sort(handles)
	return handles
}

func substitute(v any, values map[string]string) any {
	return walk(v, func(s string) string {
		if h, ok := secrets.ParseEnc(s); ok {
			return values[h]
		}
		return s
	})
}

// walk calls fn on every string value in v, replacing it with the result.
func walk(v any, fn func(string) string) any {
	switch t := v.(type) {
	case string:
		return fn(t)
	case map[string]any:
		for k, e := range t {
			t[k] = walk(e, fn)
		}
	case []any:
		for i, e := range t {
			t[i] = walk(e, fn)
		}
	}
	return v
}
