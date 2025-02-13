// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package clusterchecks

const (
	clusterChecksConfigProvider             = "clusterchecks"
	kubeServicesAndEndpointsConfigProviders = "kube_services kube_endpoints"
	kubeServicesAndEndpointsListeners       = "kube_services kube_endpoints"
	endpointsChecksConfigProvider           = "endpointschecks"
	clusterAndEndpointsConfigProviders      = "clusterchecks endpointschecks"
)
