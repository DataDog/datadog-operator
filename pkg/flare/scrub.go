// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flare

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
)

// redacted replaces every value the scrubber removes.
const redacted = "********"

var (
	// sensitiveKeyRegexp matches the names of keys, flags, environment
	// variables and env entries whose value is a credential. It matches the
	// end of the name, so references such as secretName or keyName are kept.
	sensitiveKeyRegexp = regexp.MustCompile(`(?i)(api[-_]?key|app[-_]?key|application[-_]?key|token|password|passwd|pwd|secret|credentials?|private[-_]?key|access[-_]?key|cabundle|authorization|cookie|bearer)$`)

	// jsonSensitiveRegexp scrubs `"someToken": "value"` in JSON text, such as
	// JSON log lines. The agent scrubber only handles YAML keys this way.
	jsonSensitiveRegexp = regexp.MustCompile(`(?i)("[\w.-]*(?:api_?key|app_?key|application_?key|token|password|passwd|secret|credentials?|private_?key|access_?key|authorization)"\s*:\s*)"(?:[^"\\]|\\.)*"`)
	// yamlSensitiveRegexp scrubs camelCase YAML keys such as
	// `clusterAgentToken: value`, which the agent scrubber only matches in
	// snake_case.
	yamlSensitiveRegexp = regexp.MustCompile(`(?i)^(\s*(?:-\s+)?[\w.-]*(?:api_?key|app_?key|application_?key|token|password|passwd|secret|private_?key|access_?key)\s*:)\s+[^\s{\[|>].*$`)
	// envSensitiveRegexp scrubs `NAME=value` pairs, such as environment
	// variables or command line flags, with a sensitive name.
	envSensitiveRegexp = regexp.MustCompile(`(?i)(\b[\w.-]*(?:api_?key|app_?key|application_?key|token|password|passwd|secret|private_?key|access_?key)=)\S+`)
	// certRegexp matches PEM blocks. It is the agent scrubber multi-line
	// certificate replacer.
	certRegexp = regexp.MustCompile(`-----BEGIN (?:.*)-----[A-Za-z0-9=\+\/\s]*-----END (?:.*)-----`)

	// hashValueRegexp matches the 32 or 40 hex value of a key naming a git
	// commit or a hash (`commit: <sha>`, `agentspechash: <md5>`). The agent
	// scrubber masks every such string as a possible key, so these are kept.
	hashValueRegexp = regexp.MustCompile(`(?i)(\b(?:git[ _-]?)?commit(?:[ _-]?(?:sha|id|hash))?|[\w./-]*(?:hash|checksum)(?:/[\w.-]+)?)("?\s*[:=]\s*["']?)([a-f0-9]{40}|[a-f0-9]{32})\b`)
)

// keptPlaceholder replaces a kept hash while the agent scrubber runs; no
// replacer matches it.
const keptPlaceholder = "kept-%d"

// Scrubber removes credentials from everything written to a flare. It uses
// the Datadog Agent scrubber (API and App keys, bearer tokens, passwords, URL
// credentials, certificates...) and adds the camelCase and JSON forms of
// those keys that Kubernetes objects and the Operator JSON logs use.
type Scrubber struct {
	agent *scrubber.Scrubber
}

// NewScrubber returns a Scrubber with the default replacers.
func NewScrubber() *Scrubber {
	s := scrubber.New()
	scrubber.AddDefaultReplacers(s)
	s.AddReplacer(scrubber.SingleLine, scrubber.Replacer{
		Regex: jsonSensitiveRegexp,
		Repl:  []byte(`$1"` + redacted + `"`),
	})
	s.AddReplacer(scrubber.SingleLine, scrubber.Replacer{
		Regex:    yamlSensitiveRegexp,
		ReplFunc: scrubYAMLValue,
	})
	s.AddReplacer(scrubber.SingleLine, scrubber.Replacer{
		Regex: envSensitiveRegexp,
		Repl:  []byte(`${1}` + redacted),
	})
	return &Scrubber{agent: s}
}

// scrubYAMLValue redacts the value of a sensitive YAML key, except booleans
// and numbers (such as automountServiceAccountToken: true), which can't be
// credentials.
func scrubYAMLValue(match []byte) []byte {
	sub := yamlSensitiveRegexp.FindSubmatch(match)
	if sub == nil {
		return match
	}
	value := strings.Trim(strings.TrimSpace(string(match[len(sub[1]):])), `"'`)
	switch strings.ToLower(value) {
	case "", "true", "false", "null", "~", redacted:
		return match
	}
	if _, err := strconv.ParseFloat(value, 64); err == nil && len(value) < 16 {
		return match
	}
	return append(append([]byte{}, sub[1]...), []byte(` "`+redacted+`"`)...)
}

// ScrubBytes scrubs text, line by line. Unlike the agent's ScrubBytes, it
// keeps comment lines (for example Prometheus # HELP lines) and has no line
// length limit, but scrubs them too.
func (s *Scrubber) ScrubBytes(data []byte) []byte {
	var out bytes.Buffer
	out.Grow(len(data))
	for len(data) > 0 {
		line := data
		var eol []byte
		if i := bytes.IndexByte(data, '\n'); i >= 0 {
			line, eol, data = data[:i], data[i:i+1], data[i+1:]
		} else {
			data = nil
		}
		out.WriteString(scrubLineKeepingHashes(s.agent, string(line)))
		out.Write(eol)
	}
	scrubbed := out.Bytes()
	if bytes.Contains(scrubbed, []byte("BEGIN")) {
		scrubbed = certRegexp.ReplaceAll(scrubbed, []byte(redacted))
	}
	return scrubbed
}

// scrubLineKeepingHashes scrubs a line with the agent scrubber, but keeps the
// git commits and hashes hashValueRegexp matches.
func scrubLineKeepingHashes(agent *scrubber.Scrubber, line string) string {
	var kept []string
	protected := hashValueRegexp.ReplaceAllStringFunc(line, func(match string) string {
		sub := hashValueRegexp.FindStringSubmatchIndex(match)
		kept = append(kept, match[sub[6]:sub[7]])
		return match[:sub[6]] + fmt.Sprintf(keptPlaceholder, len(kept)-1)
	})
	scrubbed := agent.ScrubLine(protected)
	for i, value := range kept {
		scrubbed = strings.Replace(scrubbed, fmt.Sprintf(keptPlaceholder, i), value, 1)
	}
	return scrubbed
}

// ScrubString scrubs a string.
func (s *Scrubber) ScrubString(data string) string {
	return string(s.ScrubBytes([]byte(data)))
}

// IsSensitiveKey reports whether a key, flag or environment variable name
// holds a credential.
func IsSensitiveKey(name string) bool {
	return sensitiveKeyRegexp.MatchString(strings.TrimSpace(name))
}

// RedactObject returns a JSON-compatible copy of obj where the string values of
// sensitive keys and of {name: DD_API_KEY, value: ...} entries are replaced.
// Object values, such as apiSecret: {secretName, keyName}, are kept.
func RedactObject(obj any) (any, error) {
	raw, err := json.Marshal(obj)
	if err != nil {
		return nil, err
	}
	var generic any
	if err := json.Unmarshal(raw, &generic); err != nil {
		return nil, err
	}
	return redactValue(generic), nil
}

func redactValue(v any) any {
	switch typed := v.(type) {
	case map[string]any:
		// Kubernetes EnvVar and similar {name, value} entries.
		if name, ok := typed["name"].(string); ok && IsSensitiveKey(name) {
			if _, ok := typed["value"].(string); ok {
				typed["value"] = redacted
			}
		}
		for key, value := range typed {
			if _, isString := value.(string); isString && IsSensitiveKey(key) {
				typed[key] = redacted
				continue
			}
			typed[key] = redactValue(value)
		}
		return typed
	case []any:
		for i := range typed {
			typed[i] = redactValue(typed[i])
		}
		return typed
	default:
		return v
	}
}
