package v1alpha2

import (
	"testing"

	"github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/api/utils"
	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	v2 "k8s.io/api/autoscaling/v2"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestNewDatadogPodAutoscalerFromV1Alpha1(t *testing.T) {
	in := &v1alpha1.DatadogPodAutoscaler{
		TypeMeta: metav1.TypeMeta{
			Kind:       "DatadogPodAutoscaler",
			APIVersion: "datadoghq.com/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-dpa",
			Namespace: "default",
		},
		Spec: v1alpha1.DatadogPodAutoscalerSpec{
			TargetRef: v2.CrossVersionObjectReference{
				Kind:       "Deployment",
				Name:       "test-deployment",
				APIVersion: "apps/v1",
			},
			Owner:         "test-owner",
			RemoteVersion: utils.NewPointer[uint64](10),
			Targets: []common.DatadogPodAutoscalerObjective{
				{
					Type: common.DatadogPodAutoscalerContainerResourceTargetType,
					PodResource: &common.DatadogPodAutoscalerResourceTarget{
						Name: v1.ResourceCPU,
						Value: common.DatadogPodAutoscalerTargetValue{
							Type:        common.DatadogPodAutoscalerUtilizationTargetValueType,
							Utilization: utils.NewPointer[int32](80),
						},
					},
				},
				{
					Type: common.DatadogPodAutoscalerPodResourceTargetType,
					PodResource: &common.DatadogPodAutoscalerResourceTarget{
						Name: v1.ResourceMemory,
						Value: common.DatadogPodAutoscalerTargetValue{
							Type:        common.DatadogPodAutoscalerUtilizationTargetValueType,
							Utilization: utils.NewPointer[int32](80),
						},
					},
				},
			},
			Constraints: &common.DatadogPodAutoscalerConstraints{
				MinReplicas: utils.NewPointer[int32](1),
				MaxReplicas: 10,
				Containers: []common.DatadogPodAutoscalerContainerConstraints{
					{
						Name:    "foo",
						Enabled: utils.NewPointer[bool](true),
						Requests: &common.DatadogPodAutoscalerContainerResourceConstraints{
							MinAllowed: v1.ResourceList{
								v1.ResourceCPU:    resource.MustParse("100m"),
								v1.ResourceMemory: resource.MustParse("100Mi"),
							},
							MaxAllowed: v1.ResourceList{
								v1.ResourceCPU:    resource.MustParse("100m"),
								v1.ResourceMemory: resource.MustParse("100Mi"),
							},
						},
					},
				},
			},
			Policy: &v1alpha1.DatadogPodAutoscalerPolicy{
				ApplyMode: v1alpha1.DatadogPodAutoscalerManualApplyMode,
				Update: &common.DatadogPodAutoscalerUpdatePolicy{
					Strategy: common.DatadogPodAutoscalerDisabledUpdateStrategy,
				},
			},
		},
	}

	expected := &DatadogPodAutoscaler{
		TypeMeta: metav1.TypeMeta{
			Kind:       "DatadogPodAutoscaler",
			APIVersion: "datadoghq.com/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-dpa",
			Namespace: "default",
		},
		Spec: DatadogPodAutoscalerSpec{
			TargetRef: v2.CrossVersionObjectReference{
				Kind:       "Deployment",
				Name:       "test-deployment",
				APIVersion: "apps/v1",
			},
			Owner:         "test-owner",
			RemoteVersion: utils.NewPointer[uint64](10),
			Objectives: []common.DatadogPodAutoscalerObjective{
				{
					Type: common.DatadogPodAutoscalerContainerResourceTargetType,
					PodResource: &common.DatadogPodAutoscalerResourceTarget{
						Name: v1.ResourceCPU,
						Value: common.DatadogPodAutoscalerTargetValue{
							Type:        common.DatadogPodAutoscalerUtilizationTargetValueType,
							Utilization: utils.NewPointer[int32](80),
						},
					},
				},
				{
					Type: common.DatadogPodAutoscalerPodResourceTargetType,
					PodResource: &common.DatadogPodAutoscalerResourceTarget{
						Name: v1.ResourceMemory,
						Value: common.DatadogPodAutoscalerTargetValue{
							Type:        common.DatadogPodAutoscalerUtilizationTargetValueType,
							Utilization: utils.NewPointer[int32](80),
						},
					},
				},
			},
			Constraints: &common.DatadogPodAutoscalerConstraints{
				MinReplicas: utils.NewPointer[int32](1),
				MaxReplicas: 10,
				Containers: []common.DatadogPodAutoscalerContainerConstraints{
					{
						Name:    "foo",
						Enabled: utils.NewPointer[bool](true),
						Requests: &common.DatadogPodAutoscalerContainerResourceConstraints{
							MinAllowed: v1.ResourceList{
								v1.ResourceCPU:    resource.MustParse("100m"),
								v1.ResourceMemory: resource.MustParse("100Mi"),
							},
							MaxAllowed: v1.ResourceList{
								v1.ResourceCPU:    resource.MustParse("100m"),
								v1.ResourceMemory: resource.MustParse("100Mi"),
							},
						},
					},
				},
			},
			ApplyPolicy: &DatadogPodAutoscalerApplyPolicy{
				Mode: DatadogPodAutoscalerApplyModePreview,
				Update: &common.DatadogPodAutoscalerUpdatePolicy{
					Strategy: common.DatadogPodAutoscalerDisabledUpdateStrategy,
				},
			},
		},
	}

	out := NewDatadogPodAutoscalerFromV1Alpha1(in)
	assert.Empty(t, cmp.Diff(expected, out))
}

func TestNewDatadogPodAutoscalerToV1Alpha1(t *testing.T) {
	in := &DatadogPodAutoscaler{
		TypeMeta: metav1.TypeMeta{
			Kind:       "DatadogPodAutoscaler",
			APIVersion: "datadoghq.com/v1alpha2",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-dpa",
			Namespace: "default",
		},
		Spec: DatadogPodAutoscalerSpec{
			TargetRef: v2.CrossVersionObjectReference{
				Kind:       "Deployment",
				Name:       "test-deployment",
				APIVersion: "apps/v1",
			},
			Owner:         "test-owner",
			RemoteVersion: utils.NewPointer[uint64](10),
			Objectives: []common.DatadogPodAutoscalerObjective{
				{
					Type: common.DatadogPodAutoscalerContainerResourceTargetType,
					PodResource: &common.DatadogPodAutoscalerResourceTarget{
						Name: v1.ResourceCPU,
						Value: common.DatadogPodAutoscalerTargetValue{
							Type:        common.DatadogPodAutoscalerUtilizationTargetValueType,
							Utilization: utils.NewPointer[int32](80),
						},
					},
				},
				{
					Type: common.DatadogPodAutoscalerPodResourceTargetType,
					PodResource: &common.DatadogPodAutoscalerResourceTarget{
						Name: v1.ResourceMemory,
						Value: common.DatadogPodAutoscalerTargetValue{
							Type:        common.DatadogPodAutoscalerUtilizationTargetValueType,
							Utilization: utils.NewPointer[int32](80),
						},
					},
				},
			},
			Constraints: &common.DatadogPodAutoscalerConstraints{
				MinReplicas: utils.NewPointer[int32](1),
				MaxReplicas: 10,
				Containers: []common.DatadogPodAutoscalerContainerConstraints{
					{
						Name:    "foo",
						Enabled: utils.NewPointer[bool](true),
						Requests: &common.DatadogPodAutoscalerContainerResourceConstraints{
							MinAllowed: v1.ResourceList{
								v1.ResourceCPU:    resource.MustParse("100m"),
								v1.ResourceMemory: resource.MustParse("100Mi"),
							},
							MaxAllowed: v1.ResourceList{
								v1.ResourceCPU:    resource.MustParse("100m"),
								v1.ResourceMemory: resource.MustParse("100Mi"),
							},
						},
					},
				},
			},
			ApplyPolicy: &DatadogPodAutoscalerApplyPolicy{
				Mode: DatadogPodAutoscalerApplyModePreview,
				Update: &common.DatadogPodAutoscalerUpdatePolicy{
					Strategy: common.DatadogPodAutoscalerDisabledUpdateStrategy,
				},
			},
		},
	}

	expected := &v1alpha1.DatadogPodAutoscaler{
		TypeMeta: metav1.TypeMeta{
			Kind:       "DatadogPodAutoscaler",
			APIVersion: "datadoghq.com/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-dpa",
			Namespace: "default",
		},
		Spec: v1alpha1.DatadogPodAutoscalerSpec{
			TargetRef: v2.CrossVersionObjectReference{
				Kind:       "Deployment",
				Name:       "test-deployment",
				APIVersion: "apps/v1",
			},
			Owner:         "test-owner",
			RemoteVersion: utils.NewPointer[uint64](10),
			Targets: []common.DatadogPodAutoscalerObjective{
				{
					Type: common.DatadogPodAutoscalerContainerResourceTargetType,
					PodResource: &common.DatadogPodAutoscalerResourceTarget{
						Name: v1.ResourceCPU,
						Value: common.DatadogPodAutoscalerTargetValue{
							Type:        common.DatadogPodAutoscalerUtilizationTargetValueType,
							Utilization: utils.NewPointer[int32](80),
						},
					},
				},
				{
					Type: common.DatadogPodAutoscalerPodResourceTargetType,
					PodResource: &common.DatadogPodAutoscalerResourceTarget{
						Name: v1.ResourceMemory,
						Value: common.DatadogPodAutoscalerTargetValue{
							Type:        common.DatadogPodAutoscalerUtilizationTargetValueType,
							Utilization: utils.NewPointer[int32](80),
						},
					},
				},
			},
			Constraints: &common.DatadogPodAutoscalerConstraints{
				MinReplicas: utils.NewPointer[int32](1),
				MaxReplicas: 10,
				Containers: []common.DatadogPodAutoscalerContainerConstraints{
					{
						Name:    "foo",
						Enabled: utils.NewPointer[bool](true),
						Requests: &common.DatadogPodAutoscalerContainerResourceConstraints{
							MinAllowed: v1.ResourceList{
								v1.ResourceCPU:    resource.MustParse("100m"),
								v1.ResourceMemory: resource.MustParse("100Mi"),
							},
							MaxAllowed: v1.ResourceList{
								v1.ResourceCPU:    resource.MustParse("100m"),
								v1.ResourceMemory: resource.MustParse("100Mi"),
							},
						},
					},
				},
			},
			Policy: &v1alpha1.DatadogPodAutoscalerPolicy{
				ApplyMode: v1alpha1.DatadogPodAutoscalerManualApplyMode,
				Update: &common.DatadogPodAutoscalerUpdatePolicy{
					Strategy: common.DatadogPodAutoscalerDisabledUpdateStrategy,
				},
			},
		},
	}

	out := NewDatadogPodAutoscalerToV1Alpha1(in)
	assert.Empty(t, cmp.Diff(expected, out))
}
