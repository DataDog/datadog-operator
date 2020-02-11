// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package plugin

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

// intToString converts int32 to string
func intToString(i int32) string {
	return fmt.Sprintf("%d", i)
}

// getDuration gets object's age
func getDuration(obj *metav1.ObjectMeta) string {
	return durafmt.ParseShort(time.Since(obj.CreationTimestamp.Time)).String()
}

// streamToBytes converts a stream to bytes
func streamToBytes(stream io.Reader) ([]byte, error) {
	bytes := new(bytes.Buffer)
	_, err := bytes.ReadFrom(stream)
	return bytes.Bytes(), err
}

// askForConfirmation asks for the user's confirmation before taking an action
func askForConfirmation(input string) bool {
	response, err := askForInput(input)
	if err != nil {
		return false
	}
	if response == "y" || response == "Y" {
		return true
	}
	return false
}

// askForInput asks the user for a given information
func askForInput(question string) (string, error) {
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
