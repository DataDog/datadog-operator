// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package constants

const (
	DDAPIKey      = "DD_API_KEY"
	DDAppKey      = "DD_APP_KEY"
	DDddURL       = "DD_DD_URL"
	DDURL         = "DD_URL"
	DDSite        = "DD_SITE"
	DDClusterName = "DD_CLUSTER_NAME"
	DDLogLevel    = "DD_LOG_LEVEL"
	DDHostName    = "DD_HOSTNAME"
	DDAPMEnabled  = "DD_APM_ENABLED"

	// Cloud-provider based (delegated) authentication to the Datadog API.
	// Names mirror the Datadog Agent "delegated_auth" configuration.
	DDDelegatedAuthOrgUUID   = "DD_DELEGATED_AUTH_ORG_UUID"
	DDDelegatedAuthProvider  = "DD_DELEGATED_AUTH_PROVIDER"
	DDDelegatedAuthAWSRegion = "DD_DELEGATED_AUTH_AWS_REGION"
)
