// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadoggenericcr

import (
	"context"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	// "github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	// "github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	// testV1 "github.com/DataDog/datadog-api-client-go/v2/tests/api/datadogV1"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
)

var (
	testLogger logr.Logger = zap.New(zap.UseDevMode(true))
)

func Test_handleFinalizer(t *testing.T) {
	s := scheme.Scheme
	s.AddKnownTypes(datadoghqv1alpha1.GroupVersion, &datadoghqv1alpha1.DatadogGenericCR{})
	metaNow := metav1.NewTime(time.Now())

	r := &Reconciler{
		client: fake.NewClientBuilder().
			WithStatusSubresource(&datadoghqv1alpha1.DatadogGenericCR{}).Build(),
		scheme: s,
		log:    testLogger,
		// datadogNotebooksClient: datadogV1.NewNotebooksApi(datadog.NewAPIClient(datadog.NewConfiguration())),
	}

	tests := []struct {
		name                  string
		gcr                   *datadoghqv1alpha1.DatadogGenericCR
		objectShouldBeDeleted bool
		finalizerShouldExist  bool
	}{
		{
			name: "a new DatadogGenericCR object gets a finalizer added successfully",
			gcr: &datadoghqv1alpha1.DatadogGenericCR{
				TypeMeta: metav1.TypeMeta{
					Kind: "DatadogGenericCR",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "test genericcr",
				},
			},
			objectShouldBeDeleted: false,
			finalizerShouldExist:  true,
		},
		// {
		// 	name: "a DatadogGenericCR object (with the finalizer) has a deletion timestamp",
		// 	gcr: &datadoghqv1alpha1.DatadogGenericCR{
		// 		TypeMeta: metav1.TypeMeta{
		// 			Kind: "DatadogGenericCR",
		// 		},
		// 		ObjectMeta: metav1.ObjectMeta{
		// 			Name:       "test genericcr",
		// 			Finalizers: []string{datadogGenericCRFinalizer},
		// 		},
		// 		Spec: datadoghqv1alpha1.DatadogGenericCRSpec{
		// 			Type: "notebook",
		// 		},
		// 	},
		// 	objectShouldBeDeleted: true,
		// 	finalizerShouldExist:  false,
		// },
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reqLogger := testLogger.WithValues("test:", test.name)
			_ = r.client.Create(context.TODO(), test.gcr)
			// Set the deletion timestamp if the test requires it as it is sanitized by the fake client with creation
			// Provide a fake ID (requires to update the status using .Status().Update)
			if test.objectShouldBeDeleted {
				test.gcr.Status.Id = "1234"
				_ = r.client.Status().Update(context.TODO(), test.gcr)
				test.gcr.SetDeletionTimestamp(&metaNow)
				_ = r.client.Update(context.TODO(), test.gcr)
			}
			_, err := r.handleFinalizer(reqLogger, test.gcr)
			assert.NoError(t, err)
			if test.finalizerShouldExist {
				assert.Contains(t, test.gcr.GetFinalizers(), datadogGenericCRFinalizer)
			} else {
				assert.NotContains(t, test.gcr.GetFinalizers(), datadogGenericCRFinalizer)
			}
		})
	}

}
