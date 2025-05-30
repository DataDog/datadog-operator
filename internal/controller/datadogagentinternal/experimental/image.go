// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package experimental

import (
	"encoding/json"

	"github.com/go-logr/logr"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/feature"
	"github.com/DataDog/datadog-operator/pkg/images"
)

type imageOverrides map[string]imageOverride

type imageOverride struct {
	// Image name to use.
	//
	// This field supports the following formats:
	// * `<NAME>` - Override the image name. The registry and tag from the original image are preserved.
	// * `<NAME>:<TAG>` - Override the entire image without specifying a registry. The default registry for underlying
	// container runtime will be used. `Tag` is ignored.
	// * `<REGISTRY>/<NAME>:<TAG>` - Override the entire image. `Tag` is ignored.
	Name string `json:"name,omitempty"`

	// Image tag to use.
	//
	// Used only when non-empty and `Name` does not specify a tag.
	Tag string `json:"tag,omitempty"`
}

func getImageOverrideConfig(ddai *v1alpha1.DatadogAgentInternal) (imageOverrides, error) {
	imageOverrideConfigRaw := getExperimentalAnnotation(ddai, ExperimentalImageOverrideConfigSubkey)
	if imageOverrideConfigRaw == "" {
		return nil, nil
	}

	imageOverrideConfig := imageOverrides{}
	if err := json.Unmarshal([]byte(imageOverrideConfigRaw), &imageOverrideConfig); err != nil {
		return nil, err
	}

	return imageOverrideConfig, nil
}

func overrideImage(currentImg string, overrideImg imageOverride) string {
	// We build an `AgentImageConfig` to feed to `overrideImage`. A little hacky, but it lets us preserve all the other
	// existing logic around image path handling.
	overrideImgConfig := &v2alpha1.AgentImageConfig{
		Name: overrideImg.Name,
		Tag:  overrideImg.Tag,
	}

	return images.OverrideAgentImage(currentImg, overrideImgConfig)
}

func applyExperimentalImageOverrides(logger logr.Logger, ddai *v1alpha1.DatadogAgentInternal, manager feature.PodTemplateManagers) {
	// We support overriding the image used for any non-init container in the Agent's pod spec.
	//
	// We grab the image override configuration from the `image-override-config` experimental annotation, and for each
	// container defined in the pod template's spec, we see if there is a defined override. If so, we apply it.
	imageOverrides, err := getImageOverrideConfig(ddai)
	if err != nil {
		logger.Error(err, "Failed to deserialize image override config")
		return
	}

	podTemplateSpec := manager.PodTemplateSpec()
	for i, container := range podTemplateSpec.Spec.Containers {
		if imageOverride, ok := imageOverrides[container.Name]; ok {
			logger.V(2).Info("Overriding container image", "container", container.Name, "image_name", imageOverride.Name, "image_tag", imageOverride.Tag)
			podTemplateSpec.Spec.Containers[i].Image = overrideImage(container.Image, imageOverride)
		}
	}
}
