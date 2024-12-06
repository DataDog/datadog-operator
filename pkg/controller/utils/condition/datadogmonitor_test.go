// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package condition

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/crds/datadoghq/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestSetErrorActiveConditions(t *testing.T) {
	now := metav1.Now()

	testCases := []struct {
		name                      string
		status                    *datadoghqv1alpha1.DatadogMonitorStatus
		err                       error
		transition                bool
		wantFirstConditionType    datadoghqv1alpha1.DatadogMonitorConditionType
		wantFirstConditionStatus  corev1.ConditionStatus
		wantSecondConditionType   datadoghqv1alpha1.DatadogMonitorConditionType
		wantSecondConditionStatus corev1.ConditionStatus
	}{
		{
			name:                     "status empty, no error",
			status:                   &datadoghqv1alpha1.DatadogMonitorStatus{},
			err:                      nil,
			wantFirstConditionType:   datadoghqv1alpha1.DatadogMonitorConditionTypeActive,
			wantFirstConditionStatus: corev1.ConditionTrue,
		},
		{
			name:                     "status empty, error",
			status:                   &datadoghqv1alpha1.DatadogMonitorStatus{},
			err:                      errors.New("dummy error"),
			wantFirstConditionType:   datadoghqv1alpha1.DatadogMonitorConditionTypeError,
			wantFirstConditionStatus: corev1.ConditionTrue,
		},
		{
			name: "status error, no error",
			status: &datadoghqv1alpha1.DatadogMonitorStatus{
				Conditions: []datadoghqv1alpha1.DatadogMonitorCondition{
					{
						Type:               datadoghqv1alpha1.DatadogMonitorConditionTypeError,
						Message:            "old description",
						Status:             corev1.ConditionTrue,
						LastTransitionTime: metav1.NewTime(now.Add(-time.Hour)),
						LastUpdateTime:     metav1.NewTime(now.Add(-time.Hour)),
					},
				},
			},
			err:                       nil,
			transition:                true,
			wantFirstConditionType:    datadoghqv1alpha1.DatadogMonitorConditionTypeError,
			wantFirstConditionStatus:  corev1.ConditionFalse,
			wantSecondConditionType:   datadoghqv1alpha1.DatadogMonitorConditionTypeActive,
			wantSecondConditionStatus: corev1.ConditionTrue,
		},
		{
			name: "status error, error",
			status: &datadoghqv1alpha1.DatadogMonitorStatus{
				Conditions: []datadoghqv1alpha1.DatadogMonitorCondition{
					{
						Type:               datadoghqv1alpha1.DatadogMonitorConditionTypeError,
						Message:            "old description",
						Status:             corev1.ConditionTrue,
						LastTransitionTime: metav1.NewTime(now.Add(-time.Hour)),
						LastUpdateTime:     metav1.NewTime(now.Add(-time.Hour)),
					},
				},
			},
			err:                      errors.New("dummy error"),
			wantFirstConditionType:   datadoghqv1alpha1.DatadogMonitorConditionTypeError,
			wantFirstConditionStatus: corev1.ConditionTrue,
		},
		{
			name: "status no error, no error",
			status: &datadoghqv1alpha1.DatadogMonitorStatus{
				Conditions: []datadoghqv1alpha1.DatadogMonitorCondition{
					{
						Type:               datadoghqv1alpha1.DatadogMonitorConditionTypeActive,
						Message:            "old description",
						Status:             corev1.ConditionTrue,
						LastTransitionTime: metav1.NewTime(now.Add(-time.Hour)),
						LastUpdateTime:     metav1.NewTime(now.Add(-time.Hour)),
					},
				},
			},
			err:                      nil,
			wantFirstConditionType:   datadoghqv1alpha1.DatadogMonitorConditionTypeActive,
			wantFirstConditionStatus: corev1.ConditionTrue,
		},
		{
			name: "status no error, error",
			status: &datadoghqv1alpha1.DatadogMonitorStatus{
				Conditions: []datadoghqv1alpha1.DatadogMonitorCondition{
					{
						Type:               datadoghqv1alpha1.DatadogMonitorConditionTypeActive,
						Message:            "old description",
						Status:             corev1.ConditionTrue,
						LastTransitionTime: metav1.NewTime(now.Add(-time.Hour)),
						LastUpdateTime:     metav1.NewTime(now.Add(-time.Hour)),
					},
				},
			},
			err:                       errors.New("dummy error"),
			transition:                true,
			wantFirstConditionType:    datadoghqv1alpha1.DatadogMonitorConditionTypeActive,
			wantFirstConditionStatus:  corev1.ConditionFalse,
			wantSecondConditionType:   datadoghqv1alpha1.DatadogMonitorConditionTypeError,
			wantSecondConditionStatus: corev1.ConditionTrue,
		},
	}

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			SetErrorActiveConditions(test.status, now, test.err)

			assert.Equal(t, test.wantFirstConditionType, test.status.Conditions[0].Type)
			assert.Equal(t, test.wantFirstConditionStatus, test.status.Conditions[0].Status)
			assert.Equal(t, now, test.status.Conditions[0].LastUpdateTime)

			if test.transition {
				assert.Equal(t, now, test.status.Conditions[0].LastTransitionTime)
			}

			switch {
			case test.err == nil && test.status.Conditions[0].Type == datadoghqv1alpha1.DatadogMonitorConditionTypeActive:
				assert.Equal(t, "DatadogMonitor ready", test.status.Conditions[0].Message)
			case test.err == nil && test.status.Conditions[0].Type == datadoghqv1alpha1.DatadogMonitorConditionTypeError:
				assert.Equal(t, "", test.status.Conditions[0].Message)
			case test.err != nil && test.status.Conditions[0].Type == datadoghqv1alpha1.DatadogMonitorConditionTypeActive:
				assert.Equal(t, "DatadogMonitor error", test.status.Conditions[0].Message)
			case test.err != nil && test.status.Conditions[0].Type == datadoghqv1alpha1.DatadogMonitorConditionTypeError:
				assert.Equal(t, test.err.Error(), test.status.Conditions[0].Message)
			}

			t.Log(test.status)

			if test.wantSecondConditionType != "" {
				assert.Equal(t, test.wantSecondConditionType, test.status.Conditions[1].Type)
				assert.Equal(t, test.wantSecondConditionStatus, test.status.Conditions[1].Status)
				assert.Equal(t, now, test.status.Conditions[1].LastUpdateTime)

				switch {
				case test.err == nil && test.status.Conditions[1].Type == datadoghqv1alpha1.DatadogMonitorConditionTypeActive:
					assert.Equal(t, "DatadogMonitor ready", test.status.Conditions[1].Message)
				case test.err == nil && test.status.Conditions[1].Type == datadoghqv1alpha1.DatadogMonitorConditionTypeError:
					assert.Equal(t, "", test.status.Conditions[1].Message)
				case test.err != nil && test.status.Conditions[1].Type == datadoghqv1alpha1.DatadogMonitorConditionTypeActive:
					assert.Equal(t, "DatadogMonitor error", test.status.Conditions[1].Message)
				case test.err != nil && test.status.Conditions[1].Type == datadoghqv1alpha1.DatadogMonitorConditionTypeError:
					assert.Contains(t, test.err.Error(), test.status.Conditions[1].Message)
				}

				if test.transition {
					assert.Equal(t, now, test.status.Conditions[1].LastTransitionTime)
				}
			}

		})
	}
}

func TestUpdateDatadogMonitorConditions(t *testing.T) {
	now := time.Now()

	testCases := []struct {
		name            string
		status          *datadoghqv1alpha1.DatadogMonitorStatus
		now             metav1.Time
		conditionType   datadoghqv1alpha1.DatadogMonitorConditionType
		conditionStatus corev1.ConditionStatus
		desc            string
		expectedStatus  *datadoghqv1alpha1.DatadogMonitorStatus
	}{
		{
			name: "conditiontype does not already exist and conditionstatus is false, do nothing",
			status: &datadoghqv1alpha1.DatadogMonitorStatus{
				Conditions: []datadoghqv1alpha1.DatadogMonitorCondition{
					{
						Type:               datadoghqv1alpha1.DatadogMonitorConditionTypeCreated,
						Message:            "old description",
						Status:             corev1.ConditionTrue,
						LastTransitionTime: metav1.NewTime(now.Add(-time.Hour)),
						LastUpdateTime:     metav1.NewTime(now.Add(-time.Hour)),
					},
				},
			},
			now:             metav1.NewTime(now),
			conditionType:   datadoghqv1alpha1.DatadogMonitorConditionTypeUpdated,
			conditionStatus: corev1.ConditionFalse,
			desc:            "new description",
			expectedStatus: &datadoghqv1alpha1.DatadogMonitorStatus{
				Conditions: []datadoghqv1alpha1.DatadogMonitorCondition{
					{
						Type:               datadoghqv1alpha1.DatadogMonitorConditionTypeCreated,
						Message:            "old description",
						Status:             corev1.ConditionTrue,
						LastTransitionTime: metav1.NewTime(now.Add(-time.Hour)),
						LastUpdateTime:     metav1.NewTime(now.Add(-time.Hour)),
					},
				},
			},
		},
		{
			name: "conditiontype does not already exist and conditionstatus is true, create new condition",
			status: &datadoghqv1alpha1.DatadogMonitorStatus{
				Conditions: []datadoghqv1alpha1.DatadogMonitorCondition{
					{
						Type:               datadoghqv1alpha1.DatadogMonitorConditionTypeCreated,
						Message:            "old description",
						Status:             corev1.ConditionTrue,
						LastTransitionTime: metav1.NewTime(now.Add(-time.Hour)),
						LastUpdateTime:     metav1.NewTime(now.Add(-time.Hour)),
					},
				},
			},
			now:             metav1.NewTime(now),
			conditionType:   datadoghqv1alpha1.DatadogMonitorConditionTypeUpdated,
			conditionStatus: corev1.ConditionTrue,
			desc:            "new description",
			expectedStatus: &datadoghqv1alpha1.DatadogMonitorStatus{
				Conditions: []datadoghqv1alpha1.DatadogMonitorCondition{
					{
						Type:               datadoghqv1alpha1.DatadogMonitorConditionTypeCreated,
						Message:            "old description",
						Status:             corev1.ConditionTrue,
						LastTransitionTime: metav1.NewTime(now.Add(-time.Hour)),
						LastUpdateTime:     metav1.NewTime(now.Add(-time.Hour)),
					},
					{
						Type:               datadoghqv1alpha1.DatadogMonitorConditionTypeUpdated,
						Message:            "new description",
						Status:             corev1.ConditionTrue,
						LastTransitionTime: metav1.NewTime(now),
						LastUpdateTime:     metav1.NewTime(now),
					},
				},
			},
		},
		{
			name: "conditiontype already exists and conditionstatus is true, update condition",
			status: &datadoghqv1alpha1.DatadogMonitorStatus{
				Conditions: []datadoghqv1alpha1.DatadogMonitorCondition{
					{
						Type:               datadoghqv1alpha1.DatadogMonitorConditionTypeUpdated,
						Message:            "old description",
						Status:             corev1.ConditionTrue,
						LastTransitionTime: metav1.NewTime(now.Add(-time.Hour)),
						LastUpdateTime:     metav1.NewTime(now.Add(-time.Hour)),
					},
				},
			},
			now:             metav1.NewTime(now),
			conditionType:   datadoghqv1alpha1.DatadogMonitorConditionTypeUpdated,
			conditionStatus: corev1.ConditionTrue,
			desc:            "new description",
			expectedStatus: &datadoghqv1alpha1.DatadogMonitorStatus{
				Conditions: []datadoghqv1alpha1.DatadogMonitorCondition{
					{
						Type:               datadoghqv1alpha1.DatadogMonitorConditionTypeUpdated,
						Message:            "new description",
						Status:             corev1.ConditionTrue,
						LastTransitionTime: metav1.NewTime(now.Add(-time.Hour)),
						LastUpdateTime:     metav1.NewTime(now),
					},
				},
			},
		},
		{
			name: "conditiontype already exists and conditionstatus is false, update condition",
			status: &datadoghqv1alpha1.DatadogMonitorStatus{
				Conditions: []datadoghqv1alpha1.DatadogMonitorCondition{
					{
						Type:               datadoghqv1alpha1.DatadogMonitorConditionTypeUpdated,
						Message:            "old description",
						Status:             corev1.ConditionTrue,
						LastTransitionTime: metav1.NewTime(now.Add(-time.Hour)),
						LastUpdateTime:     metav1.NewTime(now.Add(-time.Hour)),
					},
				},
			},
			now:             metav1.NewTime(now),
			conditionType:   datadoghqv1alpha1.DatadogMonitorConditionTypeUpdated,
			conditionStatus: corev1.ConditionFalse,
			desc:            "new description",
			expectedStatus: &datadoghqv1alpha1.DatadogMonitorStatus{
				Conditions: []datadoghqv1alpha1.DatadogMonitorCondition{
					{
						Type:               datadoghqv1alpha1.DatadogMonitorConditionTypeUpdated,
						Message:            "new description",
						Status:             corev1.ConditionFalse,
						LastTransitionTime: metav1.NewTime(now),
						LastUpdateTime:     metav1.NewTime(now),
					},
				},
			},
		},
	}
	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			UpdateDatadogMonitorConditions(test.status, test.now, test.conditionType, test.conditionStatus, test.desc)
			assert.Equal(t, test.status, test.expectedStatus)
		})
	}

}

func TestSetDatadogMonitorCondition(t *testing.T) {
	now := time.Now()

	testCases := []struct {
		name              string
		condition         *datadoghqv1alpha1.DatadogMonitorCondition
		now               metav1.Time
		conditionStatus   corev1.ConditionStatus
		desc              string
		expectedCondition *datadoghqv1alpha1.DatadogMonitorCondition
	}{
		{
			name: "condition status has not changed, only update LastUpdateTime and Message (not LastTransitionTime nor Status)",
			condition: &datadoghqv1alpha1.DatadogMonitorCondition{
				LastTransitionTime: metav1.NewTime(now.Add(-time.Hour)),
				Status:             corev1.ConditionTrue,
				LastUpdateTime:     metav1.NewTime(now.Add(-time.Hour)),
				Message:            "old description",
			},
			conditionStatus: corev1.ConditionTrue,
			desc:            "new description",
			expectedCondition: &datadoghqv1alpha1.DatadogMonitorCondition{
				LastTransitionTime: metav1.NewTime(now.Add(-time.Hour)),
				Status:             corev1.ConditionTrue,
				LastUpdateTime:     metav1.NewTime(now),
				Message:            "new description",
			},
		},
		{
			name: "condition status has changed, update LastUpdateTime, Message, LastTransitionTime and Status",
			condition: &datadoghqv1alpha1.DatadogMonitorCondition{
				LastTransitionTime: metav1.NewTime(now.Add(-time.Hour)),
				Status:             corev1.ConditionTrue,
				LastUpdateTime:     metav1.NewTime(now.Add(-time.Hour)),
				Message:            "old description",
			},
			conditionStatus: corev1.ConditionFalse,
			desc:            "new description",
			expectedCondition: &datadoghqv1alpha1.DatadogMonitorCondition{
				LastTransitionTime: metav1.NewTime(now),
				Status:             corev1.ConditionFalse,
				LastUpdateTime:     metav1.NewTime(now),
				Message:            "new description",
			},
		},
	}
	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			result := SetDatadogMonitorCondition(test.condition, metav1.NewTime(now), test.conditionStatus, test.desc)
			assert.Equal(t, test.expectedCondition, result)
		})
	}
}

func Test_getIndexForDatadogMonitorConditionType(t *testing.T) {
	testCases := []struct {
		name          string
		status        *datadoghqv1alpha1.DatadogMonitorStatus
		conditionType datadoghqv1alpha1.DatadogMonitorConditionType
		expectedIdx   int
	}{
		{
			name: "status exists in conditions",
			status: &datadoghqv1alpha1.DatadogMonitorStatus{
				Conditions: []datadoghqv1alpha1.DatadogMonitorCondition{
					{
						Type: datadoghqv1alpha1.DatadogMonitorConditionTypeCreated,
					},
				},
			},
			conditionType: datadoghqv1alpha1.DatadogMonitorConditionTypeCreated,
			expectedIdx:   0,
		},
		{
			name: "status does not exist in conditions",
			status: &datadoghqv1alpha1.DatadogMonitorStatus{
				Conditions: []datadoghqv1alpha1.DatadogMonitorCondition{
					{
						Type: datadoghqv1alpha1.DatadogMonitorConditionTypeCreated,
					},
				},
			},
			conditionType: datadoghqv1alpha1.DatadogMonitorConditionTypeUpdated,
			expectedIdx:   -1,
		},
		{
			name:          "status is nil",
			status:        nil,
			conditionType: datadoghqv1alpha1.DatadogMonitorConditionTypeUpdated,
			expectedIdx:   -1,
		},
	}
	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			idx := getIndexForDatadogMonitorConditionType(test.status, test.conditionType)
			assert.Equal(t, test.expectedIdx, idx)
		})
	}

}
