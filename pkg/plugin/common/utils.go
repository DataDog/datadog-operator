// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package common

import (
	"bufio"
	"bytes"
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
func GetDurationAsString(obj *metav1.ObjectMeta) string {
	return durafmt.ParseShort(time.Since(obj.CreationTimestamp.Time)).String()
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
		fmt.Println(question)
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
