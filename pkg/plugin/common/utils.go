// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package common

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/hako/durafmt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// IntToString converts int32 to string
func IntToString(i int32) string {
	return fmt.Sprintf("%d", i)
}

// GetDuration gets object's age
func GetDuration(obj *metav1.ObjectMeta) string {
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
