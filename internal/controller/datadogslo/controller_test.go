// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogslo

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	datadogapi "github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
	"github.com/DataDog/datadog-operator/internal/controller/utils"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
)

const (
	resourceNamespace = "default"
	resourceName      = "slo"
)

// TestReconciler_Reconcile tests the Reconcile method of the Reconciler
func TestReconciler_Reconcile(t *testing.T) {
	ctx := context.Background()
	testLogger := zap.New(zap.UseDevMode(true))
	s := scheme.Scheme
	s.AddKnownTypes(v1alpha1.GroupVersion, &v1alpha1.DatadogSLO{})

	type mockedFields struct {
		k8sClient client.Client
	}
	tests := []struct {
		name                 string
		request              ctrl.Request
		expectedResult       ctrl.Result
		mockOn               func(t *testing.T, m *mockedFields)
		datadogClientHandler http.HandlerFunc
	}{
		{
			name: "Create SLO when not exists",
			request: ctrl.Request{
				NamespacedName: types.NamespacedName{
					Namespace: resourceNamespace,
					Name:      resourceName,
				},
			},
			mockOn: func(t *testing.T, m *mockedFields) {
				_ = m.k8sClient.Create(context.TODO(), defaultSLO())
			},
			datadogClientHandler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(defaultDatadogSLOResponse())
			}),
			expectedResult: ctrl.Result{RequeueAfter: defaultRequeuePeriod},
		},
		{
			name: "Return empty result when SLO is not found",
			request: ctrl.Request{
				NamespacedName: types.NamespacedName{
					Namespace: resourceNamespace,
					Name:      resourceName,
				},
			},
			datadogClientHandler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			}),
			expectedResult: ctrl.Result{},
		},
		{
			name: "Return Error and Requeue result when creating SLO is failed",
			request: ctrl.Request{
				NamespacedName: types.NamespacedName{
					Namespace: resourceNamespace,
					Name:      resourceName,
				},
			},
			mockOn: func(t *testing.T, m *mockedFields) {
				_ = m.k8sClient.Create(context.TODO(), defaultSLO())
			},
			datadogClientHandler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, "invalid data", http.StatusBadRequest)
			}),
			expectedResult: ctrl.Result{Requeue: false, RequeueAfter: defaultErrRequeuePeriod},
		},
		{
			name: "Update SLO when exists",
			request: ctrl.Request{
				NamespacedName: types.NamespacedName{
					Namespace: resourceNamespace,
					Name:      resourceName,
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
			expectedResult: ctrl.Result{RequeueAfter: defaultRequeuePeriod},
		},
	}

	// Iterate through test cases
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			httpServer := httptest.NewServer(tt.datadogClientHandler)
			defer httpServer.Close()

			testConfig := datadogapi.NewConfiguration()
			testConfig.HTTPClient = httpServer.Client()
			apiClient := datadogapi.NewAPIClient(testConfig)
			client := datadogV1.NewServiceLevelObjectivesApi(apiClient)
			testAuth := setupTestAuth(httpServer.URL)

			m := mockedFields{
				k8sClient: fake.NewClientBuilder().WithStatusSubresource(&v1alpha1.DatadogSLO{}).Build(),
			}
			if tt.mockOn != nil {
				tt.mockOn(t, &m)
			}
			recorder := record.NewFakeRecorder(5)
			r := &Reconciler{
				client:        m.k8sClient,
				datadogClient: client,
				datadogAuth:   testAuth,
				recorder:      recorder,
				log:           testLogger,
			}

			res, _ := r.Reconcile(ctx, tt.request)
			assert.Equal(t, tt.expectedResult, res)
		})
	}
}

func defaultSLO() *v1alpha1.DatadogSLO {
	return &v1alpha1.DatadogSLO{
		TypeMeta: metav1.TypeMeta{
			Kind:       "DatadogMonitor",
			APIVersion: fmt.Sprintf("%s/%s", v1alpha1.GroupVersion.Group, v1alpha1.GroupVersion.Version),
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: resourceNamespace,
			Name:      resourceName,
		},
		Spec: v1alpha1.DatadogSLOSpec{
			Name: "Test SLO",
			Query: &v1alpha1.DatadogSLOQuery{
				Numerator:   "sum:my.custom.count.metric{type:good_events}.as_count()",
				Denominator: "sum:my.custom.count.metric{*}.as_count()",
			},
			Type:            v1alpha1.DatadogSLOTypeMetric,
			TargetThreshold: resource.MustParse("99.0"),
			Timeframe:       v1alpha1.DatadogSLOTimeFrame30d,
			Tags:            utils.GetRequiredTags(),
		},
	}
}

func defaultDatadogSLOResponse() datadogV1.SLOListResponse {
	unix := time.Date(2023, 5, 1, 0, 0, 0, 0, time.UTC).Unix()
	return datadogV1.SLOListResponse{
		Data: []datadogV1.ServiceLevelObjective{
			{
				CreatedAt: &unix,
				Creator: &datadogV1.Creator{
					Name:  *datadogapi.NewNullableString(ptrString("test user")),
					Email: ptrString("email@example.com"),
				},
				Description: datadogapi.NullableString{},
				Groups:      []string{},
				Id:          ptrString("SLO123"),
				ModifiedAt:  &unix,
				MonitorIds:  []int64{},
				MonitorTags: []string{},
				Name:        "Test",
				Query: &datadogV1.ServiceLevelObjectiveQuery{
					Denominator: "sum:my.custom.count.metric{*}.as_count()",
					Numerator:   "sum:my.custom.count.metric{type:good_events}.as_count()",
				},
				Tags: []string{"tag3", "tag4"},
				Thresholds: []datadogV1.SLOThreshold{
					{
						Timeframe: "7d",
						Target:    99,
					},
				},
				Type: "metric",
			},
		},
		Errors:   []string{},
		Metadata: &datadogV1.SLOListResponseMetadata{},
	}
}

func ptrString(s string) *string {
	return &s
}

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
