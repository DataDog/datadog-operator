// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package images

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
)

const (
	// AgentLatestVersion corresponds to the latest stable agent release
	AgentLatestVersion = "7.71.1"
	// ClusterAgentLatestVersion corresponds to the latest stable cluster-agent release
	ClusterAgentLatestVersion = "7.71.1"
	// FIPSProxyLatestVersion corresponds to the latest stable fips-proxy release
	FIPSProxyLatestVersion = "1.1.17"
	// GCRContainerRegistry corresponds to the datadoghq GCR registry
	GCRContainerRegistry = "gcr.io/datadoghq"
	// DockerHubContainerRegistry corresponds to the datadoghq docker.io registry
	DockerHubContainerRegistry = "docker.io/datadog"
	// PublicECSContainerRegistry corresponds to the datadoghq PublicECSContainerRegistry registry
	PublicECSContainerRegistry = "public.ecr.aws/datadog"
	// DefaultImageRegistry corresponds to the datadoghq containers registry
	DefaultImageRegistry = "gcr.io/datadoghq"
	// Default Image Registries
	DefaultAzureImageRegistry  string = "datadoghq.azurecr.io"
	DefaultEuropeImageRegistry string = "eu.gcr.io/datadoghq"
	DefaultAsiaImageRegistry   string = "asia.gcr.io/datadoghq"
	DefaultGovImageRegistry    string = "public.ecr.aws/datadog"
	// JMXTagSuffix tag suffix for agent JMX images
	JMXTagSuffix = "-jmx"
	// FIPSTagSuffix tag suffix for agent FIPS images
	FIPSTagSuffix = "-fips"
	// FullTagSuffix tag suffix for full agent images
	FullTagSuffix = "-full"
	// Default Image names
	DefaultAgentImageName        string = "agent"
	DefaultClusterAgentImageName string = "cluster-agent"
)

// imageHasTag identifies whether an image string contains a tag suffix
// Ref: https://github.com/distribution/distribution/blob/v2.7.1/reference/reference.go
var imageHasTag = regexp.MustCompile(`.+:[\w][\w.-]{0,127}$`)

// imageNameContainsTag return true if the image name contains a tag
func imageNameContainsTag(name string) bool {
	// The image name corresponds to a full image string
	return imageHasTag.MatchString(name)
}

// Image represents a container image information
type Image struct {
	registry string
	name     string
	tag      string
	isJMX    bool
	isFIPS   bool
	isFull   bool
}

// newImage return a new Image instance
// Assumes that tag suffixes have already been trimmed and booleans updated accordingly
func newImage(registry, name, tag string, isJMX, isFIPS, isFull bool) *Image {
	return &Image{
		registry: registry,
		name:     name,
		tag:      tag,
		isJMX:    isJMX,
		isFIPS:   isFIPS,
		isFull:   isFull,
	}
}

func (i *Image) WithRegistry(registry string) *Image {
	if registry == "" {
		return i
	}
	i.registry = registry
	return i
}

func (i *Image) WithTag(tag string) *Image {
	if tag == "" {
		return i
	}
	i.tag = tag
	return i
}

func (i *Image) WithName(name string) *Image {
	if name == "" {
		return i
	}
	i.name = name
	return i
}

func (i *Image) WithJMX(isJMX bool) *Image {
	i.isJMX = isJMX
	return i
}

func (i *Image) WithFIPS(isFIPS bool) *Image {
	i.isFIPS = isFIPS
	return i
}

func (i *Image) WithFull(isFull bool) *Image {
	i.isFull = isFull
	return i
}

// GetLatestAgentImage returns the latest stable agent release version
func GetLatestAgentImage() string {
	image := newImage(DefaultImageRegistry, DefaultAgentImageName, AgentLatestVersion, false, false, false)
	return image.ToString()
}

// GetLatestAgentImageWithSuffix returns the latest stable agent release version
func GetLatestAgentImageWithSuffix(withJMX, withFIPS, withFull bool) string {
	image := newImage(DefaultImageRegistry, DefaultAgentImageName, AgentLatestVersion, withJMX, withFIPS, withFull)
	return image.ToString()
}

// GetLatestClusterAgentImage returns the latest stable agent release version
func GetLatestClusterAgentImage() string {
	image := newImage(DefaultImageRegistry, DefaultClusterAgentImageName, ClusterAgentLatestVersion, false, false, false)
	return image.ToString()
}

// AssembleImage builds the image string based on ImageConfig and the registry configuration.
func AssembleImage(imageSpec *v2alpha1.AgentImageConfig, registry string) string {
	if imageNameContainsTag(imageSpec.Name) {
		return imageSpec.Name
	}

	if registry == "" {
		registry = DefaultImageRegistry
	}

	tag := imageSpec.Tag
	// If JMXEnabled, then proactively trim JMX suffix to prevent duplicate suffixes
	if imageSpec.JMXEnabled {
		tag = strings.TrimSuffix(imageSpec.Tag, JMXTagSuffix)
	}

	img := newImage(registry, imageSpec.Name, tag, imageSpec.JMXEnabled, false, false)

	return img.ToString()
}

// OverrideAgentImage takes an existing image reference and potentially overrides portions of it based on the provided image configuration
func OverrideAgentImage(currentImage string, overrideImageSpec *v2alpha1.AgentImageConfig) string {
	image := FromString(currentImage)
	overrideImage := fromImageConfig(overrideImageSpec)

	image.WithRegistry(overrideImage.registry).
		WithName(overrideImage.name).
		WithTag(overrideImage.tag).
		WithJMX(overrideImage.isJMX)

	// If an override tag (whether from the tag or name) is provided, then the FIPS and full suffixes present in this tag
	// should take precedence over the current image suffixes. If it's not provided, then preserve the original
	if overrideImage.tag != "" {
		image.WithFIPS(overrideImage.isFIPS).
			WithFull(overrideImage.isFull)
	}

	return image.ToString()
}

// String return the string representation of an image
func (i *Image) ToString() string {
	suffix := ""
	// FIPS is a global setting, JMX is an override setting and Full is a feature setting.
	// Order of priority is JMX -> FIPS -> Full
	if i.isJMX {
		if i.isFIPS {
			suffix = FIPSTagSuffix + JMXTagSuffix
		} else if i.isFull {
			// Since JMX is compatible with the Full image, iff isJMX and isFull are true then use the Full suffix
			suffix = FullTagSuffix
		} else {
			suffix = JMXTagSuffix

		}
	} else if i.isFIPS {
		suffix = FIPSTagSuffix
	} else if i.isFull {
		suffix = FullTagSuffix
	}

	return fmt.Sprintf("%s/%s:%s%s", i.registry, i.name, i.tag, suffix)
}

// FromString translates a string Image in the format registry/name:tag to an Image object
func FromString(stringImage string) *Image {
	splitImg := strings.Split(stringImage, "/")
	registry := strings.Join(splitImg[:len(splitImg)-1], "/")

	splitName := strings.Split(splitImg[len(splitImg)-1], ":")

	name := splitName[0]
	tag := splitName[1]

	// Check if this tag has Full suffix
	// If it does, return because it is incompatible with the other two suffixes
	isFull := strings.HasSuffix(tag, FullTagSuffix)
	if isFull {
		tag = strings.TrimSuffix(tag, FullTagSuffix)
		return newImage(registry, name, tag, false, false, isFull)
	}

	// Check if this tag has JMX or FIPS suffixes
	// JMX would be on the outside
	isJMX := strings.HasSuffix(tag, JMXTagSuffix)
	if isJMX {
		tag = strings.TrimSuffix(tag, JMXTagSuffix)
	}

	isFIPS := strings.HasSuffix(tag, FIPSTagSuffix)
	if isFIPS {
		tag = strings.TrimSuffix(tag, FIPSTagSuffix)
	}

	return newImage(registry, name, tag, isJMX, isFIPS, isFull)
}

// fromImageConfig creates an Image instance from the AgentImageConfig spec object
// We accept the imageConfig.name in the following formats:
// - name
// - name:tag
// - registry/name:tag
// (Notably, we do not accept "registry/name".)
// Note that if the name includes a tag, then we ignore imageConfig.tag and imageConfig.JMXEnabled
func fromImageConfig(imageConfig *v2alpha1.AgentImageConfig) *Image {
	registry := ""
	imageName := imageConfig.Name
	imageTag := imageConfig.Tag

	nameContainsTag := false
	isJMX := false
	isFIPS := false
	isFull := false

	if strings.Contains(imageName, ":") {
		nameContainsTag = true
		splitRes := strings.SplitN(imageName, ":", 2)
		imageName, imageTag = splitRes[0], splitRes[1]

	}
	if nameContainsTag && strings.Contains(imageName, "/") {
		lastIdx := strings.LastIndex(imageName, "/")
		registry = imageName[:lastIdx]
		imageName = imageName[lastIdx+1:]
	}

	// Check if tag has JMX suffix
	// If override name contains JMX tag, isJMX should be true
	// if override name contains non-JMX tag, isJMX should be false
	if nameContainsTag {
		isJMX = strings.HasSuffix(imageTag, JMXTagSuffix)
	} else {
		isJMX = imageConfig.JMXEnabled || strings.HasSuffix(imageTag, JMXTagSuffix)
	}

	if isJMX {
		imageTag = strings.TrimSuffix(imageTag, JMXTagSuffix)
	}

	// Check if tag has FIPS suffix
	isFIPS = strings.HasSuffix(imageTag, FIPSTagSuffix)
	if isFIPS {
		imageTag = strings.TrimSuffix(imageTag, FIPSTagSuffix)
	}

	// Check if tag has full suffix
	isFull = strings.HasSuffix(imageTag, FullTagSuffix)
	if isFull {
		imageTag = strings.TrimSuffix(imageTag, FullTagSuffix)
	}

	return newImage(registry, imageName, imageTag, isJMX, isFIPS, isFull)
}
