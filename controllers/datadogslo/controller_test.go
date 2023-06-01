// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2023 Datadog, Inc.

package datadogslo

import (
	"context"
	"encoding/json"
	"fmt"
	datadogapiclientv1 "github.com/DataDog/datadog-api-client-go/api/v1/datadog"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"time"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/tools/record"
	"net/http"
	"net/http/httptest"
	"net/url"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"testing"
)

const (
	resourcesNamespace = "default"
	resourcesName      = "slo"
)

// TestReconciler_Reconcile tests the Reconcile method of the Reconciler
func TestReconciler_Reconcile(t *testing.T) {
	ctx := context.Background()
	testLogger := zap.New(zap.UseDevMode(true))
	s := scheme.Scheme
	s.AddKnownTypes(v2alpha1.GroupVersion, &v1alpha1.DatadogSLO{})

	type mockedFields struct {
		k8sClient client.Client
	}
	tests := []struct {
		name                 string
		request              ctrl.Request
		expectedResult       ctrl.Result
		mockOn               func(t *testing.T, m *mockedFields)
		datadogClientHandler http.HandlerFunc
		expectedError        error
	}{
		{
			name: "Create SLO when not exists",
			request: ctrl.Request{
				NamespacedName: types.NamespacedName{
					Namespace: resourcesNamespace,
					Name:      resourcesName,
				},
			},
			mockOn: func(t *testing.T, m *mockedFields) {
				_ = m.k8sClient.Create(context.TODO(), defaultSLO())
			},
			datadogClientHandler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(defaultDatadogSLOResponse())
			}),
			expectedResult: ctrl.Result{},
			expectedError:  nil,
		},
		{
			name: "Return empty result when SLO is not found",
			request: ctrl.Request{
				NamespacedName: types.NamespacedName{
					Namespace: resourcesNamespace,
					Name:      resourcesName,
				},
			},
			datadogClientHandler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			}),
			expectedResult: ctrl.Result{},
			expectedError:  nil,
		},
		{
			name: "Return Error and Requeue result when creating SLO is failed",
			request: ctrl.Request{
				NamespacedName: types.NamespacedName{
					Namespace: resourcesNamespace,
					Name:      resourcesName,
				},
			},
			mockOn: func(t *testing.T, m *mockedFields) {
				_ = m.k8sClient.Create(context.TODO(), defaultSLO())
			},
			datadogClientHandler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, "invalid data", http.StatusBadRequest)
			}),
			expectedResult: ctrl.Result{Requeue: true, RequeueAfter: defaultErrRequeuePeriod},
			expectedError:  fmt.Errorf("error creating monitor: 400 Bad Request: invalid data\n"),
		},
		{
			name: "Update SLO when exists",
			request: ctrl.Request{
				NamespacedName: types.NamespacedName{
					Namespace: resourcesNamespace,
					Name:      resourcesName,
				},
			},
			mockOn: func(t *testing.T, m *mockedFields) {
				slo := defaultSLO()
				slo.Status.ID = "SLO123"
				_ = m.k8sClient.Create(context.TODO(), slo)
			},
			datadogClientHandler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(defaultDatadogSLOResponse())
			}),
			expectedResult: ctrl.Result{},
			expectedError:  nil,
		},
	}

	// Iterate through test single test cases
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			httpServer := httptest.NewServer(tt.datadogClientHandler)
			defer httpServer.Close()

			testConfig := datadogapiclientv1.NewConfiguration()
			testConfig.HTTPClient = httpServer.Client()
			ddClient := datadogapiclientv1.NewAPIClient(testConfig)
			testAuth := setupTestAuth(httpServer.URL)

			m := mockedFields{
				k8sClient: fake.NewClientBuilder().Build(),
			}
			if tt.mockOn != nil {
				tt.mockOn(t, &m)
			}
			recorder := record.NewFakeRecorder(5)
			r := &Reconciler{
				client:        m.k8sClient,
				datadogClient: ddClient,
				datadogAuth:   testAuth,
				recorder:      recorder,
				log:           testLogger,
				versionInfo:   &version.Info{},
			}

			res, err := r.Reconcile(ctx, tt.request)
			assert.Equal(t, tt.expectedResult, res)

			if tt.expectedError != nil {
				assert.EqualError(t, tt.expectedError, err.Error())
			}
		})
	}
}

func defaultSLO() *v1alpha1.DatadogSLO {
	return &v1alpha1.DatadogSLO{
		TypeMeta: metav1.TypeMeta{
			Kind:       "DatadogMonitor",
			APIVersion: fmt.Sprintf("%s/%s", v2alpha1.GroupVersion.Group, v2alpha1.GroupVersion.Version),
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: resourcesNamespace,
			Name:      resourcesName,
		},
		Spec: v1alpha1.DatadogSLOSpec{
			Name: "Test SLO",
			Query: &v1alpha1.DatadogSLOQuery{
				Numerator:   "sum:my.custom.count.metric{type:good_events}.as_count()",
				Denominator: "sum:my.custom.count.metric{*}.as_count()",
			},
			Type: v1alpha1.DatadogSLOTypeMetric,
			Thresholds: []v1alpha1.DatadogSLOThreshold{
				{
					Target:    resource.MustParse("99.0"),
					Timeframe: v1alpha1.DatadogSLOTimeFrame30d,
				},
			},
		},
	}
}

func defaultDatadogSLOResponse() datadogapiclientv1.SLOListResponse {
	unix := time.Date(2023, 5, 1, 0, 0, 0, 0, time.UTC).Unix()
	return datadogapiclientv1.SLOListResponse{
		Data: []datadogapiclientv1.ServiceLevelObjective{
			{
				CreatedAt: &unix,
				Creator: &datadogapiclientv1.Creator{
					Name:  *datadogapiclientv1.NewNullableString(ptrString("test user")),
					Email: ptrString("email@example.com"),
				},
				Description: datadogapiclientv1.NullableString{},
				Groups:      []string{},
				Id:          ptrString("SLO123"),
				ModifiedAt:  &unix,
				MonitorIds:  []int64{},
				MonitorTags: []string{},
				Name:        "Test",
				Query: &datadogapiclientv1.ServiceLevelObjectiveQuery{
					Denominator: "sum:my.custom.count.metric{*}.as_count()",
					Numerator:   "sum:my.custom.count.metric{type:good_events}.as_count()",
				},
				Tags: []string{"tag3", "tag4"},
				Thresholds: []datadogapiclientv1.SLOThreshold{
					{
						Timeframe: "7d",
						Target:    99,
					},
				},
				Type: "metric",
			},
		},
		Errors:   []string{},
		Metadata: &datadogapiclientv1.SLOListResponseMetadata{},
	}
}

func ptrString(s string) *string {
	return &s
}

func setupTestAuth(apiURL string) context.Context {
	testAuth := context.WithValue(
		context.Background(),
		datadogapiclientv1.ContextAPIKeys,
		map[string]datadogapiclientv1.APIKey{
			"apiKeyAuth": {
				Key: "DUMMY_API_KEY",
			},
			"appKeyAuth": {
				Key: "DUMMY_APP_KEY",
			},
		},
	)
	parsedAPIURL, _ := url.Parse(apiURL)
	testAuth = context.WithValue(testAuth, datadogapiclientv1.ContextServerIndex, 1)
	testAuth = context.WithValue(testAuth, datadogapiclientv1.ContextServerVariables, map[string]string{
		"name":     parsedAPIURL.Host,
		"protocol": parsedAPIURL.Scheme,
	})

	return testAuth
}
