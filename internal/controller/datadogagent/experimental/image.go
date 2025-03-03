// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package experimental

import (
	"fmt"
	"strings"

	"github.com/go-logr/logr"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
)

func imageOverrideAnnotationSubkey(containerName string) string {
	return fmt.Sprintf("%s.%s", ExperimentalImageOverrideSuffix, containerName)
}

func getImageOverrideValue(dda *v2alpha1.DatadogAgent, containerName string) string {
	// The annotation for overriding a container's image is specified in the form of `image-override.<container-name>:
	// <image>` where the container name is the name of the container in the pod spec, so `trace-agent` for the Trace
	// Agent, and so on.
	return getExperimentalAnnotation(dda, imageOverrideAnnotationSubkey(containerName))
}

func overrideImage(currentImg, overrideImg string) string {
	// We build an `AgentImageConfig` to feed to `overrideImage`. A little hacky, but it lets us preserve all the other
	// existing logic around image path handling.
	overrideName := ""
	overrideTag := ""

	splitImg := strings.Split(overrideImg, ":")
	if len(splitImg) == 1 {
		overrideName = splitImg[0]
	} else if len(splitImg) == 2 {
		overrideName = splitImg[0]
		overrideTag = splitImg[1]
	} else {
		// Image has more than one colon, or none, which is wrong, so we just use the current image.
		return currentImg
	}

	overrideImgConfig := &v2alpha1.AgentImageConfig{
		Name: overrideName,
		Tag:  overrideTag,
	}

	return common.OverrideAgentImage(currentImg, overrideImgConfig)
}

func processExperimentalImageOverrides(logger logr.Logger, dda *v2alpha1.DatadogAgent, manager feature.PodTemplateManagers) {
	// We support overriding the image used for any non-init container in the Agent's pod spec. For each container that
	// is defined for the pod, we check if iti has an image override annotation. If it does, we update the container's
	// image to the value specified in the annotation.
	for i, container := range manager.PodTemplateSpec().Spec.Containers {
		imageOverride := getImageOverrideValue(dda, container.Name)
		if imageOverride != "" {
			logger.V(2).Info("Overriding container image", "container", container.Name, "image", imageOverride)
			manager.PodTemplateSpec().Spec.Containers[i].Image = overrideImage(container.Image, imageOverride)
		}
	}
}
