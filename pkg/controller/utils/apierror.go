// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"errors"
	"net/http"
)

// APIError wraps a Datadog API client error with its HTTP status code.
// StatusCode is 0 if no response was received (e.g. a network failure).
type APIError struct {
	err        error
	StatusCode int
}

// NewAPIError wraps err with httpResp's status code, if any. Returns nil if err is nil.
func NewAPIError(err error, httpResp *http.Response) error {
	if err == nil {
		return nil
	}
	apiErr := &APIError{err: err}
	if httpResp != nil {
		apiErr.StatusCode = httpResp.StatusCode
	}
	return apiErr
}

func (e *APIError) Error() string { return e.err.Error() }

func (e *APIError) Unwrap() error { return e.err }

// IsPermanentAPIError reports whether err is a 4xx (client-side, non-retryable)
// error, excluding 429 which is worth retrying after a backoff.
func IsPermanentAPIError(err error) bool {
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	return apiErr.StatusCode >= 400 && apiErr.StatusCode < 500 && apiErr.StatusCode != http.StatusTooManyRequests
}
