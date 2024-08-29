// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import (
	"fmt"
	"strings"

	commonv1 "github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"
)

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
