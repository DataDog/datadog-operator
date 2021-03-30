// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package flare

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"regexp"
	"strings"
)

// replacer structure to store regex matching and replacement functions
type replacer struct {
	regex    *regexp.Regexp
	hints    []string // If any of these hints do not exist in the line, then we know the regex wont match either
	repl     []byte
	replFunc func(b []byte) []byte
}

var (
	commentRegex                            = regexp.MustCompile(`^\s*#.*$`)
	blankRegex                              = regexp.MustCompile(`^\s*$`)
	singleLineReplacers, multiLineReplacers []replacer
)

func init() {
	apiKeyReplacer := replacer{
		regex: regexp.MustCompile(`\b[a-fA-F0-9]{27}([a-fA-F0-9]{5})\b`),
		repl:  []byte(`***************************$1`),
	}
	appKeyReplacer := replacer{
		regex: regexp.MustCompile(`\b[a-fA-F0-9]{35}([a-fA-F0-9]{5})\b`),
		repl:  []byte(`***********************************$1`),
	}
	uriPasswordReplacer := replacer{
		regex: regexp.MustCompile(`([A-Za-z]+\:\/\/|\b)([A-Za-z0-9_]+)\:([^\s-]+)\@`),
		repl:  []byte(`$1$2:********@`),
	}
	passwordReplacer := replacer{
		regex: matchYAMLKeyPart(`(pass(word)?|pwd)`),
		hints: []string{"pass", "pwd"},
		repl:  []byte(`$1 ********`),
	}
	tokenReplacer := replacer{
		regex: matchYAMLKeyPart(`token`),
		hints: []string{"token"},
		repl:  []byte(`$1 ********`),
	}
	certReplacer := replacer{
		regex: matchCert(),
		hints: []string{"BEGIN"},
		repl:  []byte(`********`),
	}
	singleLineReplacers = []replacer{apiKeyReplacer, appKeyReplacer, uriPasswordReplacer, passwordReplacer, tokenReplacer}
	multiLineReplacers = []replacer{certReplacer}
}

func matchYAMLKeyPart(part string) *regexp.Regexp {
	return regexp.MustCompile(fmt.Sprintf(`(\s*(\w|_)*%s(\w|_)*\s*:).+`, part))
}

func matchCert() *regexp.Regexp {
	/*
	   Try to match as accurately as possible RFC 7468's ABNF
	   Backreferences are not available in go, so we cannot verify
	   here that the BEGIN label is the same as the END label.
	*/
	return regexp.MustCompile(
		`-----BEGIN (?:.*)-----[A-Za-z0-9=\+\/\s]*-----END (?:.*)-----`,
	)
}

// credentialsCleanerBytes scrubs credentials from slice of bytes
func credentialsCleanerBytes(data []byte) ([]byte, error) {
	r := bytes.NewReader(data)
	return credentialsCleaner(r)
}

func credentialsCleaner(file io.Reader) ([]byte, error) {
	var cleanedFile []byte

	scanner := bufio.NewScanner(file)

	// First, we go through the file line by line, applying any
	// single-line replacer that matches the line.
	first := true
	for scanner.Scan() {
		b := scanner.Bytes()
		if !commentRegex.Match(b) && !blankRegex.Match(b) && string(b) != "" {
			for _, repl := range singleLineReplacers {
				containsHint := false
				for _, hint := range repl.hints {
					if strings.Contains(string(b), hint) {
						containsHint = true
						break
					}
				}
				if len(repl.hints) == 0 || containsHint {
					if repl.replFunc != nil {
						b = repl.regex.ReplaceAllFunc(b, repl.replFunc)
					} else {
						b = repl.regex.ReplaceAll(b, repl.repl)
					}
				}
			}
			if !first {
				cleanedFile = append(cleanedFile, byte('\n'))
			}

			cleanedFile = append(cleanedFile, b...)
			first = false
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	// Then we apply multiline replacers on the cleaned file
	for _, repl := range multiLineReplacers {
		containsHint := false
		for _, hint := range repl.hints {
			if strings.Contains(string(cleanedFile), hint) {
				containsHint = true
				break
			}
		}
		if len(repl.hints) == 0 || containsHint {
			if repl.replFunc != nil {
				cleanedFile = repl.regex.ReplaceAllFunc(cleanedFile, repl.replFunc)
			} else {
				cleanedFile = repl.regex.ReplaceAll(cleanedFile, repl.repl)
			}
		}
	}

	return cleanedFile, nil
}
