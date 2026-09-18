// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogsyntheticsprivatelocation

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	datadogapi "github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	ctrutils "github.com/DataDog/datadog-operator/pkg/controller/utils"
)

func setupTestAuth(apiURL string) context.Context {
	testAuth := context.WithValue(
		context.Background(),
		datadogapi.ContextAPIKeys,
		map[string]datadogapi.APIKey{
			"apiKeyAuth": {
				Key: "DUMMY_API_KEY",
			},
			"appKeyAuth": {
				Key: "DUMMY_APP_KEY",
			},
		},
	)
	parsedAPIURL, _ := url.Parse(apiURL)
	testAuth = context.WithValue(testAuth, datadogapi.ContextServerIndex, 1)
	testAuth = context.WithValue(testAuth, datadogapi.ContextServerVariables, map[string]string{
		"name":     parsedAPIURL.Host,
		"protocol": parsedAPIURL.Scheme,
	})

	return testAuth
}

func newTestSyntheticsClient(httpServer *httptest.Server) *datadogV1.SyntheticsApi {
	testConfig := datadogapi.NewConfiguration()
	testConfig.HTTPClient = httpServer.Client()
	apiClient := datadogapi.NewAPIClient(testConfig)
	return datadogV1.NewSyntheticsApi(apiClient)
}

func newTestInstance() *datadoghqv1alpha1.DatadogSyntheticsPrivateLocation {
	return &datadoghqv1alpha1.DatadogSyntheticsPrivateLocation{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-pl",
			Namespace: "default",
		},
		Spec: datadoghqv1alpha1.DatadogSyntheticsPrivateLocationSpec{
			Name:        "my private location",
			Description: "some description",
			Tags:        []string{"team:foo"},
		},
	}
}

func Test_buildPrivateLocation(t *testing.T) {
	tests := []struct {
		name       string
		instance   *datadoghqv1alpha1.DatadogSyntheticsPrivateLocation
		expectedPL datadogV1.SyntheticsPrivateLocation
	}{
		{
			name:     "default: required tag appended and sorted",
			instance: newTestInstance(),
			expectedPL: datadogV1.SyntheticsPrivateLocation{
				Name:        "my private location",
				Description: "some description",
				Tags:        []string{"generated:kubernetes", "team:foo"},
			},
		},
		{
			name: "required tag already present",
			instance: func() *datadoghqv1alpha1.DatadogSyntheticsPrivateLocation {
				i := newTestInstance()
				i.Spec.Tags = []string{"generated:kubernetes", "team:foo"}
				return i
			}(),
			expectedPL: datadogV1.SyntheticsPrivateLocation{
				Name:        "my private location",
				Description: "some description",
				Tags:        []string{"generated:kubernetes", "team:foo"},
			},
		},
		{
			name: "required tag disabled",
			instance: func() *datadoghqv1alpha1.DatadogSyntheticsPrivateLocation {
				i := newTestInstance()
				i.Spec.ControllerOptions = &datadoghqv1alpha1.DatadogSPLControllerOptions{
					DisableRequiredTags: true,
				}
				return i
			}(),
			expectedPL: datadogV1.SyntheticsPrivateLocation{
				Name:        "my private location",
				Description: "some description",
				Tags:        []string{"team:foo"},
			},
		},
		{
			name: "id set from status",
			instance: func() *datadoghqv1alpha1.DatadogSyntheticsPrivateLocation {
				i := newTestInstance()
				i.Status.ID = "pl-id-123"
				return i
			}(),
			expectedPL: datadogV1.SyntheticsPrivateLocation{
				Name:        "my private location",
				Description: "some description",
				Tags:        []string{"generated:kubernetes", "team:foo"},
				Id:          func() *string { s := "pl-id-123"; return &s }(),
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			pl := buildPrivateLocation(test.instance)
			assert.Equal(t, &test.expectedPL, pl)
		})
	}
}

func Test_createPrivateLocation(t *testing.T) {
	instance := newTestInstance()
	expectedResp := datadogV1.SyntheticsPrivateLocationCreationResponse{
		PrivateLocation: &datadogV1.SyntheticsPrivateLocation{
			Name:        "my private location",
			Description: "some description",
			Tags:        []string{"generated:kubernetes", "team:foo"},
			Id:          func() *string { s := "pl-id-123"; return &s }(),
		},
	}
	jsonResp, _ := expectedResp.MarshalJSON()
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(jsonResp)
	}))
	defer httpServer.Close()

	client := newTestSyntheticsClient(httpServer)
	resp, err := createPrivateLocation(setupTestAuth(httpServer.URL), client, instance)
	assert.Nil(t, err)
	assert.Equal(t, &expectedResp, resp)
}

func Test_createPrivateLocationError(t *testing.T) {
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"errors": ["Forbidden"]}`))
	}))
	defer httpServer.Close()

	client := newTestSyntheticsClient(httpServer)
	resp, err := createPrivateLocation(setupTestAuth(httpServer.URL), client, newTestInstance())
	assert.Nil(t, resp)
	assert.Error(t, err)
	var apiErr *ctrutils.APIError
	assert.True(t, errors.As(err, &apiErr))
	assert.Equal(t, http.StatusForbidden, apiErr.StatusCode)
	assert.True(t, ctrutils.IsPermanentAPIError(err))
}

func Test_getPrivateLocation(t *testing.T) {
	expectedPL := datadogV1.SyntheticsPrivateLocation{
		Name:        "my private location",
		Description: "some description",
		Tags:        []string{"generated:kubernetes", "team:foo"},
		Id:          func() *string { s := "pl-id-123"; return &s }(),
	}
	jsonPL, _ := expectedPL.MarshalJSON()
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(jsonPL)
	}))
	defer httpServer.Close()

	client := newTestSyntheticsClient(httpServer)
	pl, err := getPrivateLocation(setupTestAuth(httpServer.URL), client, "pl-id-123")
	assert.Nil(t, err)
	assert.Equal(t, &expectedPL, pl)
}

func Test_getPrivateLocationNotFound(t *testing.T) {
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"errors": ["Private Location not found"]}`))
	}))
	defer httpServer.Close()

	client := newTestSyntheticsClient(httpServer)
	pl, err := getPrivateLocation(setupTestAuth(httpServer.URL), client, "pl-id-123")
	assert.Nil(t, err)
	assert.Nil(t, pl)
}

func Test_getPrivateLocationError(t *testing.T) {
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"errors": ["Unauthorized"]}`))
	}))
	defer httpServer.Close()

	client := newTestSyntheticsClient(httpServer)
	pl, err := getPrivateLocation(setupTestAuth(httpServer.URL), client, "pl-id-123")
	assert.Nil(t, pl)
	assert.Error(t, err)
	var apiErr *ctrutils.APIError
	assert.True(t, errors.As(err, &apiErr))
	assert.Equal(t, http.StatusUnauthorized, apiErr.StatusCode)
}

func Test_updatePrivateLocation(t *testing.T) {
	instance := newTestInstance()
	instance.Status.ID = "pl-id-123"
	expectedPL := datadogV1.SyntheticsPrivateLocation{
		Name:        "my private location",
		Description: "some description",
		Tags:        []string{"generated:kubernetes", "team:foo"},
		Id:          func() *string { return &instance.Status.ID }(),
	}
	jsonPL, _ := expectedPL.MarshalJSON()
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(jsonPL)
	}))
	defer httpServer.Close()

	client := newTestSyntheticsClient(httpServer)
	pl, err := updatePrivateLocation(setupTestAuth(httpServer.URL), client, instance)
	assert.Nil(t, err)
	assert.Equal(t, &expectedPL, pl)
}

func Test_deletePrivateLocation(t *testing.T) {
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer httpServer.Close()

	client := newTestSyntheticsClient(httpServer)
	err := deletePrivateLocation(setupTestAuth(httpServer.URL), client, "pl-id-123")
	assert.Nil(t, err)
}

func Test_deletePrivateLocationNotFound(t *testing.T) {
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"errors": ["Private Location not found"]}`))
	}))
	defer httpServer.Close()

	client := newTestSyntheticsClient(httpServer)
	// 404 is a success for idempotent finalization.
	err := deletePrivateLocation(setupTestAuth(httpServer.URL), client, "pl-id-123")
	assert.Nil(t, err)
}

func Test_deletePrivateLocationError(t *testing.T) {
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"errors": ["Forbidden"]}`))
	}))
	defer httpServer.Close()

	client := newTestSyntheticsClient(httpServer)
	err := deletePrivateLocation(setupTestAuth(httpServer.URL), client, "pl-id-123")
	assert.Error(t, err)
	var apiErr *ctrutils.APIError
	assert.True(t, errors.As(err, &apiErr))
	assert.Equal(t, http.StatusForbidden, apiErr.StatusCode)
}
