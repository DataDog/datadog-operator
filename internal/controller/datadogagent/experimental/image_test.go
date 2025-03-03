// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package experimental

import (
	"fmt"
	"testing"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/fake"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func TestImageOverride(t *testing.T) {
	defaultContainerRegistry := "docker.io/datadog"
	defaultImageName := "agent"
	defaultImageTag := "7.63.0"
	defaultContainerImage := fmt.Sprintf("%s/%s:%s", defaultContainerRegistry, defaultImageName, defaultImageTag)
	overrideImageTagged := "override-image-tagged:1.2.3"
	overrideImageUntagged := "override-image-untagged"

	tests := []struct {
		name              string
		containerName     apicommon.AgentContainerName
		configureDda      func(dda *v2alpha1.DatadogAgent, containerName string)
		validateContainer func(t *testing.T, container *v1.Container)
	}{
		{
			name:          "no override",
			containerName: apicommon.TraceAgentContainerName,
			configureDda:  func(dda *v2alpha1.DatadogAgent, containerName string) {},
			validateContainer: func(t *testing.T, container *v1.Container) {
				assert.Equal(t, defaultContainerImage, container.Image)
			},
		},
		{
			name:          "override with tagged name",
			containerName: apicommon.TraceAgentContainerName,
			configureDda: func(dda *v2alpha1.DatadogAgent, containerName string) {
				annotationKey := getExperimentalAnnotationKey(imageOverrideAnnotationSubkey(containerName))
				dda.Annotations[annotationKey] = overrideImageTagged
			},
			validateContainer: func(t *testing.T, container *v1.Container) {
				expected := fmt.Sprintf("%s/%s", defaultContainerRegistry, overrideImageTagged)
				assert.Equal(t, expected, container.Image)
			},
		},
		{
			name:          "override with untagged name",
			containerName: apicommon.TraceAgentContainerName,
			configureDda: func(dda *v2alpha1.DatadogAgent, containerName string) {
				annotationKey := getExperimentalAnnotationKey(imageOverrideAnnotationSubkey(containerName))
				dda.Annotations[annotationKey] = overrideImageUntagged
			},
			validateContainer: func(t *testing.T, container *v1.Container) {
				expected := fmt.Sprintf("%s/%s:%s", defaultContainerRegistry, overrideImageUntagged, defaultImageTag)
				assert.Equal(t, expected, container.Image)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			testLogger := zap.New(zap.UseDevMode(true))
			logger := testLogger.WithValues("test", t.Name())

			manager := fake.NewPodTemplateManagers(t, v1.PodTemplateSpec{
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name:  string(test.containerName),
							Image: defaultContainerImage,
						},
					},
				},
			})

			dda := &v2alpha1.DatadogAgent{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{},
				},
			}
			test.configureDda(dda, string(test.containerName))

			processExperimentalImageOverrides(logger, dda, manager)

			var testContainer v1.Container
			for _, container := range manager.PodTemplateSpec().Spec.Containers {
				if container.Name == string(test.containerName) {
					testContainer = container
					break
				}
			}

			assert.NotNil(t, testContainer, "container %s not found in PodTemplateManager", test.containerName)

			test.validateContainer(t, &testContainer)
		})
	}
}
