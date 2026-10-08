// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package datadogbyoccluster holds the RBAC markers of the DatadogBYOCCluster controller. They are kept out of
// the controller package so they are generated into a dedicated ClusterRole (config/rbac/byoc) instead of
// manager-role, letting deployments grant BYOC permissions only when the controller is enabled.
package datadogbyoccluster

// +kubebuilder:rbac:groups=datadoghq.com,resources=datadogbyocclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=datadoghq.com,resources=datadogbyocclusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=datadoghq.com,resources=datadogbyocclusters/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=configmaps;serviceaccounts;services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=deployments;statefulsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=autoscaling,resources=horizontalpodautoscalers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=policy,resources=poddisruptionbudgets,verbs=get;list;watch;create;update;patch;delete
