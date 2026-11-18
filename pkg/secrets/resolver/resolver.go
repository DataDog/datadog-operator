// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package resolver resolves ENC[] references in JSON using the Agent secrets component.
package resolver

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"sync"

	secrets "github.com/DataDog/datadog-agent/comp/core/secrets/def"
	secretsimpl "github.com/DataDog/datadog-agent/comp/core/secrets/impl"
	secretsutils "github.com/DataDog/datadog-agent/comp/core/secrets/utils"
	noopsimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl/noops"
	agentlog "github.com/DataDog/datadog-agent/pkg/util/log"
	"sigs.k8s.io/yaml"

	operatorsecrets "github.com/DataDog/datadog-operator/pkg/secrets"
)

// ErrUnresolved is returned when a payload references secrets that can't be resolved.
var ErrUnresolved = errors.New("unable to resolve secret references")

// Disable Agent logging to prevent unbounded buffering and raw stderr output.
var setupLogger = sync.OnceFunc(func() { agentlog.SetupLogger(agentlog.Disabled(), "off") })

// handlesKey wraps the handles sent to the Agent, which registers it as a scrubber key.
const handlesKey = "datadoggenericresource_secret_handles"

// Resolver resolves whole-string ENC[] references in JSON values.
type Resolver struct {
	comp       secrets.Component
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

// New returns a Resolver; configErr blocks references if a backend is configured.
func New(params secrets.ConfigParams, configErr error) *Resolver {
	setupLogger()
	if params.Timeout == 0 {
		params.Timeout = int(operatorsecrets.DefaultCmdTimeout.Seconds())
	}
	if params.MaxSize == 0 {
		params.MaxSize = operatorsecrets.DefaultCmdOutputMaxSize
	}
	if params.RunPath == "" {
		params.RunPath = os.TempDir()
	}
	if params.Command != "" {
		params.Type, params.Config = "", nil
	} else {
		// Arguments belong to the command; the embedded connector takes none.
		params.Arguments = nil
	}
	comp := secretsimpl.NewEnabledResolver(noopsimpl.GetCompatComponent())
	comp.Configure(params)
	return &Resolver{
		comp:       comp,
		configErr:  configErr,
		configured: params.Command != "" || params.Type != "" || len(params.MultiBackends) > 0,
	}
}

// HasReferences reports whether jsonSpec contains whole-string secret references.
func HasReferences(jsonSpec string) bool {
	if !strings.Contains(jsonSpec, "ENC[") {
		return false
	}
	tree, ok := decode(jsonSpec)
	return ok && len(findHandles(tree)) > 0
}

// ResolveJSON replaces references in jsonSpec with their values. Errors name handles, never values.
func (r *Resolver) ResolveJSON(jsonSpec, origin, namespace string) (Result, error) {
	unchanged := Result{JSON: jsonSpec}
	if !strings.Contains(jsonSpec, "ENC[") {
		return unchanged, nil
	}
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

	values, err := r.fetch(handles, origin, namespace)
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

// fetch resolves handles via the Agent, sending only the references, not the payload.
func (r *Resolver) fetch(handles []string, origin, namespace string) (map[string]string, error) {
	values := make(map[string]string, len(handles))
	var left []string
	for _, h := range handles {
		ref := "ENC[" + h + "]"
		in, err := json.Marshal(map[string][]string{handlesKey: {ref}})
		if err != nil {
			return nil, unresolvedError(handles)
		}
		// Resolve separately so a cached ENC[] value cannot be substituted again.
		out, _ := r.comp.Resolve(in, origin, "", namespace, false)
		var parsed map[string][]string
		if out == nil || yaml.Unmarshal(out, &parsed) != nil || len(parsed[handlesKey]) != 1 || parsed[handlesKey][0] == ref {
			left = append(left, h)
			continue
		}
		values[h] = parsed[handlesKey][0]
	}
	if len(left) > 0 {
		return nil, unresolvedError(left)
	}
	return values, nil
}

func unresolvedError(handles []string) error {
	return fmt.Errorf("%w: %s", ErrUnresolved, strings.Join(handles, ", "))
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
		if ok, h := secretsutils.IsEnc(s); ok && !slices.Contains(handles, h) {
			handles = append(handles, h)
		}
		return s
	})
	slices.Sort(handles)
	return handles
}

func substitute(v any, values map[string]string) any {
	return walk(v, func(s string) string {
		if ok, h := secretsutils.IsEnc(s); ok {
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
