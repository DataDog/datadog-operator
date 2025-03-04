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

type imageOverrideTestDef struct {
	name string
	tag  string
}

func (i *imageOverrideTestDef) ImageOverrideJSON(containerName string) string {
	imageOverrides := imageOverrides{
		containerName: imageOverride{
			Name: i.name,
			Tag:  i.tag,
		},
	}

	json, err := imageOverrides.JSON()
	if err != nil {
		panic(err)
	}

	return json
}

func nameOnlyImageOverride(name string) *imageOverrideTestDef {
	return &imageOverrideTestDef{
		name: name,
		tag:  "",
	}
}

func tagOnlyImageOverride(tag string) *imageOverrideTestDef {
	return &imageOverrideTestDef{
		name: "",
		tag:  tag,
	}
}

func nameAndTagImageOverride(name, tag string) *imageOverrideTestDef {
	return &imageOverrideTestDef{
		name: name,
		tag:  tag,
	}
}

func fullImageOverride(registry, name, tag string) *imageOverrideTestDef {
	return &imageOverrideTestDef{
		name: fmt.Sprintf("%s/%s", registry, name),
		tag:  tag,
	}
}

func TestImageOverride(t *testing.T) {
	defaultContainerRegistry := "docker.io/datadog"
	defaultImageName := "agent"
	defaultImageTag := "7.63.0"
	defaultContainerImage := fmt.Sprintf("%s/%s:%s", defaultContainerRegistry, defaultImageName, defaultImageTag)
	overrideContainerRegistry := "myregistry.io"
	overrideImageTagged := "override-image-tagged:1.2.3"
	overrideImageUntagged := "override-image-untagged"
	overrideImageTag := "9.9.9"

	tests := []struct {
		name              string
		containerName     apicommon.AgentContainerName
		imageOverrideDef  *imageOverrideTestDef
		validateContainer func(t *testing.T, container *v1.Container, imageOverrideDef *imageOverrideTestDef)
	}{
		{
			name:             "no override",
			containerName:    apicommon.TraceAgentContainerName,
			imageOverrideDef: nil,
			validateContainer: func(t *testing.T, container *v1.Container, imageOverrideDef *imageOverrideTestDef) {
				assert.Equal(t, defaultContainerImage, container.Image)
			},
		},
		{
			name:             "override with tagged name",
			containerName:    apicommon.TraceAgentContainerName,
			imageOverrideDef: nameOnlyImageOverride(overrideImageTagged),
			validateContainer: func(t *testing.T, container *v1.Container, imageOverrideDef *imageOverrideTestDef) {
				// When using a _name_ that has both the name and tag specified, it replaces the original image entirely.
				assert.Equal(t, imageOverrideDef.name, container.Image)
			},
		},
		{
			name:             "override with untagged name",
			containerName:    apicommon.TraceAgentContainerName,
			imageOverrideDef: nameOnlyImageOverride(overrideImageUntagged),
			validateContainer: func(t *testing.T, container *v1.Container, imageOverrideDef *imageOverrideTestDef) {
				// When only the name is overridden, and it's not tagged and there's no registry specified, it ends up
				// as an update to the name portion of the original image, so registry and/or tag are preserved.
				expected := fmt.Sprintf("%s/%s:%s", defaultContainerRegistry, imageOverrideDef.name, defaultImageTag)
				assert.Equal(t, expected, container.Image)
			},
		},
		{
			name:             "override with tag specified",
			containerName:    apicommon.TraceAgentContainerName,
			imageOverrideDef: tagOnlyImageOverride(overrideImageTag),
			validateContainer: func(t *testing.T, container *v1.Container, imageOverrideDef *imageOverrideTestDef) {
				// When only the tag is overridden, it ends as an update to the tag portion of the original image, so
				// the registry and/or name are preserved.
				expected := fmt.Sprintf("%s/%s:%s", defaultContainerRegistry, defaultImageName, imageOverrideDef.tag)
				assert.Equal(t, expected, container.Image)
			},
		},
		{
			name:             "override with tagged name and tag specified",
			containerName:    apicommon.TraceAgentContainerName,
			imageOverrideDef: nameAndTagImageOverride(overrideImageTagged, overrideImageTag),
			validateContainer: func(t *testing.T, container *v1.Container, imageOverrideDef *imageOverrideTestDef) {
				// When using a _name_ that has both the name and tag specified, it replaces the original image
				// entirely... even if we also specify a tag.
				assert.Equal(t, imageOverrideDef.name, container.Image)
			},
		},
		{
			name:             "override with untagged name and tag specified",
			containerName:    apicommon.TraceAgentContainerName,
			imageOverrideDef: nameAndTagImageOverride(overrideImageUntagged, overrideImageTag),
			validateContainer: func(t *testing.T, container *v1.Container, imageOverrideDef *imageOverrideTestDef) {
				// When only the name is overridden (but untagged), and there's also a tag specified, it ends up as an
				// update to the name and tag of the original image, so registry is preserved.
				expected := fmt.Sprintf("%s/%s:%s", defaultContainerRegistry, imageOverrideDef.name, imageOverrideDef.tag)
				assert.Equal(t, expected, container.Image)
			},
		},
		{
			name:             "override with tagged name with registry",
			containerName:    apicommon.TraceAgentContainerName,
			imageOverrideDef: fullImageOverride(overrideContainerRegistry, overrideImageTagged, ""),
			validateContainer: func(t *testing.T, container *v1.Container, imageOverrideDef *imageOverrideTestDef) {
				// When specifying a full image -- <registry or repository>/name :tag -- the name replaces the original
				// image entirely.
				assert.Equal(t, imageOverrideDef.name, container.Image)
			},
		},
		{
			name:             "override with untagged name with registry",
			containerName:    apicommon.TraceAgentContainerName,
			imageOverrideDef: fullImageOverride(overrideContainerRegistry, overrideImageUntagged, ""),
			validateContainer: func(t *testing.T, container *v1.Container, imageOverrideDef *imageOverrideTestDef) {
				// When specifying a "full" image without a tag -- <registry or repository>/name -- it ends up as an
				// update to the name of the original image, so the registry and tag are preserved.
				expected := fmt.Sprintf("%s/%s:%s", defaultContainerRegistry, imageOverrideDef.name, defaultImageTag)
				assert.Equal(t, expected, container.Image)
			},
		},
		{
			name:             "override with tagged name with registry and tag",
			containerName:    apicommon.TraceAgentContainerName,
			imageOverrideDef: fullImageOverride(overrideContainerRegistry, overrideImageTagged, overrideImageTag),
			validateContainer: func(t *testing.T, container *v1.Container, imageOverrideDef *imageOverrideTestDef) {
				// When specifying a full image -- <registry or repository>/name:tag -- the name replaces the original
				// image entirely... even when we specify a tag separately.
				assert.Equal(t, imageOverrideDef.name, container.Image)
			},
		},
		{
			name:             "override with untagged name with registry and tag",
			containerName:    apicommon.TraceAgentContainerName,
			imageOverrideDef: fullImageOverride(overrideContainerRegistry, overrideImageUntagged, overrideImageTag),
			validateContainer: func(t *testing.T, container *v1.Container, imageOverrideDef *imageOverrideTestDef) {
				// When specifying a full image -- <registry or repository>/name -- and there's also a tag specified, it
				// ends up as an update to the name and tag of the original image, so the registry is preserved.
				expected := fmt.Sprintf("%s/%s:%s", defaultContainerRegistry, imageOverrideDef.name, imageOverrideDef.tag)
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
			dda := &v2alpha1.DatadogAgent{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{},
				},
			}

			if test.imageOverrideDef != nil {
				imageOverrideConfig := test.imageOverrideDef.ImageOverrideJSON(containerName)
				annotationKey := getExperimentalAnnotationKey(ExperimentalImageOverrideConfigSubkey)
				dda.Annotations[annotationKey] = imageOverrideConfig
			}

			applyExperimentalImageOverrides(logger, dda, manager)

			var testContainer v1.Container
			for _, container := range manager.PodTemplateSpec().Spec.Containers {
				if container.Name == containerName {
					testContainer = container
					break
				}
			}

			assert.NotNil(t, testContainer, "container %s not found in PodTemplateManager", test.containerName)

			test.validateContainer(t, &testContainer, test.imageOverrideDef)
		})
	}
}
