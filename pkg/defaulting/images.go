// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package defaulting

import (
	"fmt"
	"regexp"
	"strings"
)

// ContainerRegistry represent a container registry URL
type ContainerRegistry string

const (
	// AgentLatestVersion corresponds to the latest stable agent release
	AgentLatestVersion = "7.64.3"
	// ClusterAgentLatestVersion corresponds to the latest stable cluster-agent release
	ClusterAgentLatestVersion = "7.64.3"
	// FIPSProxyLatestVersion corresponds to the latest stable fips-proxy release
	FIPSProxyLatestVersion = "1.1.9"
	// GCRContainerRegistry corresponds to the datadoghq GCR registry
	GCRContainerRegistry ContainerRegistry = "gcr.io/datadoghq"
	// DockerHubContainerRegistry corresponds to the datadoghq docker.io registry
	DockerHubContainerRegistry ContainerRegistry = "docker.io/datadog"
	// PublicECSContainerRegistry corresponds to the datadoghq PublicECSContainerRegistry registry
	PublicECSContainerRegistry ContainerRegistry = "public.ecr.aws/datadog"
	// DefaultImageRegistry corresponds to the datadoghq containers registry
	DefaultImageRegistry = "gcr.io/datadoghq"
	// Default Image Registries
	DefaultAzureImageRegistry  string = "datadoghq.azurecr.io"
	DefaultEuropeImageRegistry string = "eu.gcr.io/datadoghq"
	DefaultAsiaImageRegistry   string = "asia.gcr.io/datadoghq"
	DefaultGovImageRegistry    string = "public.ecr.aws/datadog"
	// JMXTagSuffix suffix tag for agent JMX images
	JMXTagSuffix = "-jmx"
	// Default Image names
	DefaultAgentImageName        string = "agent"
	DefaultClusterAgentImageName string = "cluster-agent"
	OTelAgentBetaTag                    = "7.63.0-ot-beta-jmx"
)

// imageHasTag identifies whether an image string contains a tag suffix
// Ref: https://github.com/distribution/distribution/blob/v2.7.1/reference/reference.go
var imageHasTag = regexp.MustCompile(`.+:[\w][\w.-]{0,127}$`)

// IsImageNameContainsTag return true if the image name contains a tag
func IsImageNameContainsTag(name string) bool {
	// The image name corresponds to a full image string
	return imageHasTag.MatchString(name)
}

// Image represents a container image information
type Image struct {
	registry  ContainerRegistry
	imageName string
	tag       string
	isJMX     bool
}

// NewImage return a new Image instance
func NewImage(name, tag string, isJMX bool) *Image {
	return &Image{
		registry:  DefaultImageRegistry,
		imageName: name,
		tag:       strings.TrimSuffix(tag, JMXTagSuffix),
		isJMX:     strings.HasSuffix(tag, JMXTagSuffix) || isJMX,
	}
}

// ImageOptions use to allow extra Image configuration
type ImageOptions func(*Image)

// GetLatestAgentImage return the latest stable agent release version
func GetLatestAgentImage(opts ...ImageOptions) string {
	image := &Image{
		registry:  DefaultImageRegistry,
		imageName: DefaultAgentImageName,
		tag:       AgentLatestVersion,
	}
	processOptions(image, opts...)
	return image.String()
}

// GetLatestClusterAgentImage return the latest stable agent release version
func GetLatestClusterAgentImage(opts ...ImageOptions) string {
	image := &Image{
		registry:  DefaultImageRegistry,
		imageName: DefaultClusterAgentImageName,
		tag:       ClusterAgentLatestVersion,
	}
	processOptions(image, opts...)
	return image.String()
}

// WithRegistry ImageOptions to specify the container registry
func WithRegistry(registry ContainerRegistry) ImageOptions {
	return func(image *Image) {
		image.registry = registry
	}
}

// WithTag ImageOptions to specify the container image tag.
func WithTag(tag string) ImageOptions {
	return func(image *Image) {
		image.tag = tag
	}
}

// WithImageName ImageOptions to specify the image name
func WithImageName(name string) ImageOptions {
	return func(image *Image) {
		image.imageName = name
	}
}

func processOptions(image *Image, opts ...ImageOptions) {
	for _, option := range opts {
		option(image)
	}
}

// String return the string representation of an image
func (i *Image) String() string {
	suffix := ""
	if i.isJMX {
		suffix = JMXTagSuffix
	}
	return fmt.Sprintf("%s/%s:%s%s", i.registry, i.imageName, i.tag, suffix)
}
