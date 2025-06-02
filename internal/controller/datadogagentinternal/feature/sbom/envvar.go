// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sbom

const (
	DDSBOMEnabled                      = "DD_SBOM_ENABLED"
	DDSBOMContainerImageEnabled        = "DD_SBOM_CONTAINER_IMAGE_ENABLED"
	DDSBOMContainerImageAnalyzers      = "DD_SBOM_CONTAINER_IMAGE_ANALYZERS"
	DDSBOMContainerUseMount            = "DD_SBOM_CONTAINER_IMAGE_USE_MOUNT"
	DDSBOMContainerOverlayFSDirectScan = "DD_SBOM_CONTAINER_IMAGE_OVERLAYFS_DIRECT_SCAN"
	DDSBOMHostEnabled                  = "DD_SBOM_HOST_ENABLED"
	DDSBOMHostAnalyzers                = "DD_SBOM_HOST_ANALYZERS"
)
