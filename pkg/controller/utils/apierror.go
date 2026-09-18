// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"

	datadogapi "github.com/DataDog/datadog-api-client-go/v2/api/datadog"
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

// TranslateClientError wraps a Datadog API client error in an APIError,
// unwrapping GenericOpenAPIError and url.Error into a readable message.
func TranslateClientError(err error, httpResp *http.Response, msg string) error {
	if msg == "" {
		msg = "an error occurred"
	}

	var apiErr datadogapi.GenericOpenAPIError
	var errURL *url.Error
	if errors.As(err, &apiErr) {
		return NewAPIError(fmt.Errorf(msg+": %w: %s", err, apiErr.Body()), httpResp)
	}

	if errors.As(err, &errURL) {
		return NewAPIError(fmt.Errorf(msg+" (url.Error): %s", errURL), httpResp)
	}

	return NewAPIError(fmt.Errorf(msg+": %w", err), httpResp)
}

// TranslateUnmarshalError wraps a failure to unmarshal a spec as a permanent,
// bad-request-equivalent error: no HTTP request was made, and the spec will
// never parse until it's fixed, so it's classified the same way a 400 from
// the API would be instead of being treated as transient.
func TranslateUnmarshalError(err error, msg string) error {
	return TranslateClientError(err, &http.Response{StatusCode: http.StatusBadRequest}, msg)
}
