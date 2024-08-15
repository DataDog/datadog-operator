// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import (
	"errors"
	"fmt"
	"strings"

	commonv1 "github.com/DataDog/datadog-operator/api/datadoghq/common/v1"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"

	"k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes"
)

// IsV2Available returns true if the v2alpha1.DatadogAgent resource kind is available
func IsV2Available(cl *kubernetes.Clientset) (bool, error) {
	_, resources, err := cl.Discovery().ServerGroupsAndResources()
	if err != nil {
		if !discovery.IsGroupDiscoveryFailedError(err) {
			return false, fmt.Errorf("unable to perform resource discovery: %w", err)
		}
		var errGroup *discovery.ErrGroupDiscoveryFailed
		if errors.As(err, &errGroup) {
			for group, apiGroupErr := range errGroup.Groups {
				return false, fmt.Errorf("unable to perform resource discovery for group %s: %w", group, apiGroupErr)
			}
		}
	}

	for _, resourceGroup := range resources {
		if resourceGroup.GroupVersion == "datadoghq.com/v2alpha1" {
			for _, resource := range resourceGroup.APIResources {
				if resource.Kind == "DatadogAgent" {
					return true, nil
				}
			}
		}
	}
	return false, nil
}

func OverrideComponentImage(spec *v2alpha1.DatadogAgentSpec, cmpName v2alpha1.ComponentName, imageName, imageTag string) error {
	if _, found := spec.Override[cmpName]; !found {
		spec.Override[cmpName] = &v2alpha1.DatadogAgentComponentOverride{
			Image: &commonv1.AgentImageConfig{
				Name: imageName,
				Tag:  imageTag,
			},
		}
		return nil
	}
	cmpOverride := spec.Override[cmpName]
	if !apiutils.BoolValue(cmpOverride.Disabled) {

		if cmpOverride.Image == nil {
			cmpOverride.Image = &commonv1.AgentImageConfig{}
		}
		if cmpOverride.Image.Name == imageName && cmpOverride.Image.Tag == imageTag {
			return fmt.Errorf("the current nodeAgent image is already %s:%s", imageName, imageTag)
		}
		cmpOverride.Image.Name = imageName
		cmpOverride.Image.Tag = imageTag
	}
	return nil
}

func SplitImageString(in string) (name string, tag string) {
	imageSplit := strings.Split(in, ":")
	if len(imageSplit) > 0 {
		name = imageSplit[0]
	}
	if len(imageSplit) > 1 {
		tag = imageSplit[1]
	}
	return name, tag
}
