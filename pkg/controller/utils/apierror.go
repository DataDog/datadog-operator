// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"errors"
	"net/http"
)

// APIError wraps an error returned by a Datadog API client call together with
// the HTTP status code of the response that produced it, so callers can
// classify the failure without re-parsing error strings. StatusCode is 0 when
// no response was ever received (e.g. a network-level failure or timeout).
type APIError struct {
	err        error
	StatusCode int
}

// NewAPIError wraps err with the status code from httpResp, if any. httpResp
// may be nil, in which case the wrapped error carries no status code and is
// treated as transient by IsPermanentAPIError. Returns nil if err is nil.
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

// IsPermanentAPIError reports whether err is a client-side (4xx) Datadog API
// error that will keep failing on retry until the resource's spec changes,
// e.g. a validation failure on an invalid query or field. 429 (rate limited)
// is treated as transient, since backing off resolves it without any spec
// change. Errors that carry no status code (network failures, timeouts) and
// errors that were never wrapped with a status code are treated as transient,
// since there is no basis to conclude they are permanent.
func IsPermanentAPIError(err error) bool {
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	return apiErr.StatusCode >= 400 && apiErr.StatusCode < 500 && apiErr.StatusCode != http.StatusTooManyRequests
}
