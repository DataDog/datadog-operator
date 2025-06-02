// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package experimental

import (
	"fmt"
	"testing"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/feature/fake"
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

	tests := []struct {
		name                string
		containerName       apicommon.AgentContainerName
		imageOverrideConfig string
		validateContainer   func(t *testing.T, container *v1.Container)
	}{
		{
			name:                "no override",
			containerName:       apicommon.TraceAgentContainerName,
			imageOverrideConfig: "",
			validateContainer: func(t *testing.T, container *v1.Container) {
				assert.Equal(t, defaultContainerImage, container.Image)
			},
		},
		{
			name:                "override with tagged name",
			containerName:       apicommon.TraceAgentContainerName,
			imageOverrideConfig: `{"trace-agent":{"name":"override-image-tagged:1.2.3"}}`,
			validateContainer: func(t *testing.T, container *v1.Container) {
				// When using a _name_ that has both the name and tag specified, it replaces the original image entirely.
				assert.Equal(t, "docker.io/datadog/override-image-tagged:1.2.3", container.Image)
			},
		},
		{
			name:                "override with untagged name",
			containerName:       apicommon.TraceAgentContainerName,
			imageOverrideConfig: `{"trace-agent":{"name":"override-image-untagged"}}`,
			validateContainer: func(t *testing.T, container *v1.Container) {
				// When only the name is overridden, and it's not tagged and there's no registry specified, it ends up
				// as an update to the name portion of the original image, so registry and/or tag are preserved.
				expected := fmt.Sprintf("%s/override-image-untagged:%s", defaultContainerRegistry, defaultImageTag)
				assert.Equal(t, expected, container.Image)
			},
		},
		{
			name:                "override with tag specified",
			containerName:       apicommon.TraceAgentContainerName,
			imageOverrideConfig: `{"trace-agent":{"tag":"9.9.9"}}`,
			validateContainer: func(t *testing.T, container *v1.Container) {
				// When only the tag is overridden, it ends as an update to the tag portion of the original image, so
				// the registry and/or name are preserved.
				expected := fmt.Sprintf("%s/%s:9.9.9", defaultContainerRegistry, defaultImageName)
				assert.Equal(t, expected, container.Image)
			},
		},
		{
			name:                "override with tagged name and tag specified",
			containerName:       apicommon.TraceAgentContainerName,
			imageOverrideConfig: `{"trace-agent":{"name":"override-image-tagged:1.2.3","tag":"9.9.9"}}`,
			validateContainer: func(t *testing.T, container *v1.Container) {
				// When using a _name_ that has both the name and tag specified, it replaces the original image
				// entirely... even if we also specify a tag.
				assert.Equal(t, "docker.io/datadog/override-image-tagged:1.2.3", container.Image)
			},
		},
		{
			name:                "override with untagged name and tag specified",
			containerName:       apicommon.TraceAgentContainerName,
			imageOverrideConfig: `{"trace-agent":{"name":"override-image-untagged","tag":"9.9.9"}}`,
			validateContainer: func(t *testing.T, container *v1.Container) {
				// When only the name is overridden (but untagged), and there's also a tag specified, it ends up as an
				// update to the name and tag of the original image, so registry is preserved.
				expected := fmt.Sprintf("%s/override-image-untagged:9.9.9", defaultContainerRegistry)
				assert.Equal(t, expected, container.Image)
			},
		},
		{
			name:                "override with tagged name with registry",
			containerName:       apicommon.TraceAgentContainerName,
			imageOverrideConfig: `{"trace-agent":{"name":"myregistry.io/override-image-tagged:1.2.3"}}`,
			validateContainer: func(t *testing.T, container *v1.Container) {
				// When specifying a full image -- <registry or repository>/name :tag -- the name replaces the original
				// image entirely.
				assert.Equal(t, "myregistry.io/override-image-tagged:1.2.3", container.Image)
			},
		},
		{
			name:                "override with untagged name with registry",
			containerName:       apicommon.TraceAgentContainerName,
			imageOverrideConfig: `{"trace-agent":{"name":"override-repo/override-image-untagged"}}`,
			validateContainer: func(t *testing.T, container *v1.Container) {
				// When specifying a "full" image without a tag -- <registry or repository>/name -- it ends up as an
				// update to the name of the original image, so the registry and tag are preserved.
				expected := fmt.Sprintf("%s/override-repo/override-image-untagged:%s", defaultContainerRegistry, defaultImageTag)
				assert.Equal(t, expected, container.Image)
			},
		},
		{
			name:                "override with tagged name with registry and tag",
			containerName:       apicommon.TraceAgentContainerName,
			imageOverrideConfig: `{"trace-agent":{"name":"myregistry.io/override-image-tagged:1.2.3","tag":"9.9.9"}}`,
			validateContainer: func(t *testing.T, container *v1.Container) {
				// When specifying a full image -- <registry or repository>/name:tag -- the name replaces the original
				// image entirely... even when we specify a tag separately.
				assert.Equal(t, "myregistry.io/override-image-tagged:1.2.3", container.Image)
			},
		},
		{
			name:                "override with untagged name with registry and tag",
			containerName:       apicommon.TraceAgentContainerName,
			imageOverrideConfig: `{"trace-agent":{"name":"override-repo/override-image-untagged","tag":"9.9.9"}}`,
			validateContainer: func(t *testing.T, container *v1.Container) {
				// When specifying a full image -- <registry or repository>/name -- and there's also a tag specified, it
				// ends up as an update to the name and tag of the original image, so the registry is preserved.
				expected := fmt.Sprintf("%s/override-repo/override-image-untagged:9.9.9", defaultContainerRegistry)
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

			containerName := string(test.containerName)

			// Build the `DatadogAgent` resource and apply the image override configuration, if one was specified.
			ddai := &v1alpha1.DatadogAgentInternal{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{},
				},
			}

			if test.imageOverrideConfig != "" {
				annotationKey := getExperimentalAnnotationKey(ExperimentalImageOverrideConfigSubkey)
				ddai.Annotations[annotationKey] = test.imageOverrideConfig
			}

			applyExperimentalImageOverrides(logger, ddai, manager)

			var testContainer v1.Container
			for _, container := range manager.PodTemplateSpec().Spec.Containers {
				if container.Name == containerName {
					testContainer = container
					break
				}
			}

			assert.NotNil(t, testContainer, "container %s not found in PodTemplateManager", test.containerName)

			test.validateContainer(t, &testContainer)
		})
	}
}
