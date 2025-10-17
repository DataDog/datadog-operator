// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/hako/durafmt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// IntToString converts int32 to string
func IntToString(i int32) string {
	return fmt.Sprintf("%d", i)
}

// GetDurationAsString gets object's age
func GetDurationAsString(obj metav1.Object) string {
	return durafmt.ParseShort(time.Since(obj.GetCreationTimestamp().Time)).String()
}

// StreamToBytes converts a stream to bytes
func StreamToBytes(stream io.Reader) ([]byte, error) {
	bytes := new(bytes.Buffer)
	_, err := bytes.ReadFrom(stream)

	return bytes.Bytes(), err
}

// AskForConfirmation asks for the user's confirmation before taking an action
func AskForConfirmation(input string) bool {
	response, err := AskForInput(input)
	if err != nil {
		return false
	}

	if response == "y" || response == "Y" {
		return true
	}

	return false
}

// AskForInput asks the user for a given information
func AskForInput(question string) (string, error) {
	scanner := bufio.NewScanner(os.Stdin)
	if question != "" {
		fmt.Println(question) //nolint:forbidigo
	}

	scanner.Scan()
	text := scanner.Text()
	if err := scanner.Err(); err != nil {
		return "", err
	}

	return strings.TrimSpace(text), nil
}

// HasImagePattern returns true if the image string respects the format <string>/<string>:<string>
func HasImagePattern(image string) bool {
	matched, _ := regexp.MatchString(`.+\/.+:.+`, image)

	return matched
}

// ValidateUpgrade valides the input of the upgrade commands
func ValidateUpgrade(image string, latest bool) error {
	if image != "" && !HasImagePattern(image) {
		return fmt.Errorf("image %s doesn't respect the format <account>/<repo>:<tag>", image)
	}

	if image == "" && !latest {
		return errors.New("both 'image' and 'latest' flags are missing")
	}

	return nil
}

// IsAnnotated returns true if annotations contain a key with a given prefix
func IsAnnotated(annotations map[string]string, prefix string) bool {
	if prefix == "" || annotations == nil {
		return false
	}

	for k := range annotations {
		if strings.HasPrefix(k, prefix) {
			return true
		}
	}

	return false
}

// ValidateAnnotationsContent reports errors in AD annotations content
// the identifier string is expected to include the AD prefix
func ValidateAnnotationsContent(annotations map[string]string, identifier string) ([]string, bool) {
	if !IsAnnotated(annotations, identifier) {
		return []string{}, false
	}

	errors := []string{}
	adAnnotations := map[string]bool{
		// Required
		fmt.Sprintf("%s.check_names", identifier):  true,
		fmt.Sprintf("%s.init_configs", identifier): true,
		fmt.Sprintf("%s.instances", identifier):    true,
		// Optional
		fmt.Sprintf("%s.logs", identifier): false,
		fmt.Sprintf("%s.tags", identifier): false,
	}

	metricAnnotations := false
	for annotation, required := range adAnnotations {
		if _, found := annotations[annotation]; found && required {
			metricAnnotations = true
			break
		}
	}

	for annotation, required := range adAnnotations {
		value, found := annotations[annotation]
		if !found && required && metricAnnotations {
			errors = append(errors, fmt.Sprintf("Annotation %s is missing", annotation))
			continue
		}
		if !found {
			continue
		}
		var unmarshalled any
		if err := json.Unmarshal([]byte(value), &unmarshalled); err != nil {
			errors = append(errors, fmt.Sprintf("Annotation %s with value %s is not a valid JSON: %v", annotation, value, err))
		}
	}

	return errors, true
}

// ValidateAnnotationsMatching detects if AD annotations don't match a valid container identifier
func ValidateAnnotationsMatching(annotations map[string]string, validIDs map[string]bool) []string {
	errors := []string{}
	for annotation := range annotations {
		if matched, _ := regexp.MatchString(fmt.Sprintf(`%s.+\..+`, ADPrefixRegex), annotation); matched {
			id := strings.Split(annotation[len(ADPrefix):], ".")[0]
			if found := validIDs[id]; !found {
				errors = append(errors, fmt.Sprintf("Annotation %s is invalid: %s doesn't match a container name", annotation, id))
			}
		}
	}

	return errors
}
