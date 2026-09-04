// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_NewAPIError(t *testing.T) {
	t.Run("nil error returns nil", func(t *testing.T) {
		assert.Nil(t, NewAPIError(nil, &http.Response{StatusCode: 400}))
	})

	t.Run("nil response is preserved as a status-less error", func(t *testing.T) {
		err := NewAPIError(errors.New("network error"), nil)
		assert.EqualError(t, err, "network error")

		var apiErr *APIError
		assert.True(t, errors.As(err, &apiErr))
		assert.Equal(t, 0, apiErr.StatusCode)
	})

	t.Run("wrapped error unwraps to the original error", func(t *testing.T) {
		wrapped := fmt.Errorf("wrapping: %w", errors.New("root cause"))
		err := NewAPIError(wrapped, &http.Response{StatusCode: 400})
		assert.True(t, errors.Is(err, wrapped))
	})
}

func Test_IsPermanentAPIError(t *testing.T) {
	testCases := []struct {
		name       string
		err        error
		wantResult bool
	}{
		{
			name:       "nil error is not permanent",
			err:        nil,
			wantResult: false,
		},
		{
			name:       "plain error with no status code is not permanent",
			err:        errors.New("boom"),
			wantResult: false,
		},
		{
			name:       "APIError with no response (network failure) is not permanent",
			err:        NewAPIError(errors.New("boom"), nil),
			wantResult: false,
		},
		{
			name:       "400 is permanent",
			err:        NewAPIError(errors.New("bad request"), &http.Response{StatusCode: http.StatusBadRequest}),
			wantResult: true,
		},
		{
			name:       "404 is permanent",
			err:        NewAPIError(errors.New("not found"), &http.Response{StatusCode: http.StatusNotFound}),
			wantResult: true,
		},
		{
			name:       "422 is permanent",
			err:        NewAPIError(errors.New("unprocessable"), &http.Response{StatusCode: http.StatusUnprocessableEntity}),
			wantResult: true,
		},
		{
			name:       "429 (rate limited) is transient, not permanent",
			err:        NewAPIError(errors.New("rate limited"), &http.Response{StatusCode: http.StatusTooManyRequests}),
			wantResult: false,
		},
		{
			name:       "500 is transient, not permanent",
			err:        NewAPIError(errors.New("server error"), &http.Response{StatusCode: http.StatusInternalServerError}),
			wantResult: false,
		},
		{
			name:       "503 is transient, not permanent",
			err:        NewAPIError(errors.New("unavailable"), &http.Response{StatusCode: http.StatusServiceUnavailable}),
			wantResult: false,
		},
		{
			name:       "wrapped APIError is still detected through fmt.Errorf",
			err:        fmt.Errorf("outer: %w", NewAPIError(errors.New("bad request"), &http.Response{StatusCode: http.StatusBadRequest})),
			wantResult: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.wantResult, IsPermanentAPIError(tc.err))
		})
	}
}
