// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package kubernetes

const (
	// AppKubernetesNameLabelKey The name of the application
	AppKubernetesNameLabelKey = "app.kubernetes.io/name"
	// AppKubernetesInstanceLabelKey A unique name identifying the instance of an application
	AppKubernetesInstanceLabelKey = "app.kubernetes.io/instance"
	// AppKubernetesVersionLabelKey The current version of the application
	AppKubernetesVersionLabelKey = "app.kubernetes.io/version"
	// AppKubernetesComponentLabelKey The component within the architecture
	AppKubernetesComponentLabelKey = "app.kubernetes.io/component"
	// AppKubernetesPartOfLabelKey The name of a higher level application this one is part of
	AppKubernetesPartOfLabelKey = "app.kubernetes.io/part-of"
	// AppKubernetesManageByLabelKey The tool being used to manage the operation of an application
	AppKubernetesManageByLabelKey = "app.kubernetes.io/managed-by"
)
