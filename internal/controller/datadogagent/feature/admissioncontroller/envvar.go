// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package admissioncontroller

const (
	DDAdmissionControllerAgentSidecarEnabled             = "DD_ADMISSION_CONTROLLER_AGENT_SIDECAR_ENABLED"
	DDAdmissionControllerAgentSidecarClusterAgentEnabled = "DD_ADMISSION_CONTROLLER_AGENT_SIDECAR_CLUSTER_AGENT_ENABLED"
	DDAdmissionControllerAgentSidecarProvider            = "DD_ADMISSION_CONTROLLER_AGENT_SIDECAR_PROVIDER"
	DDAdmissionControllerAgentSidecarRegistry            = "DD_ADMISSION_CONTROLLER_AGENT_SIDECAR_CONTAINER_REGISTRY"
	DDAdmissionControllerAgentSidecarImageName           = "DD_ADMISSION_CONTROLLER_AGENT_SIDECAR_IMAGE_NAME"
	DDAdmissionControllerAgentSidecarImageTag            = "DD_ADMISSION_CONTROLLER_AGENT_SIDECAR_IMAGE_TAG"
	DDAdmissionControllerAgentSidecarSelectors           = "DD_ADMISSION_CONTROLLER_AGENT_SIDECAR_SELECTORS"
	DDAdmissionControllerAgentSidecarProfiles            = "DD_ADMISSION_CONTROLLER_AGENT_SIDECAR_PROFILES"
	DDAdmissionControllerEnabled                         = "DD_ADMISSION_CONTROLLER_ENABLED"
	DDAdmissionControllerValidationEnabled               = "DD_ADMISSION_CONTROLLER_VALIDATION_ENABLED"
	DDAdmissionControllerMutationEnabled                 = "DD_ADMISSION_CONTROLLER_MUTATION_ENABLED"
	DDAdmissionControllerInjectConfig                    = "DD_ADMISSION_CONTROLLER_INJECT_CONFIG_ENABLED"
	DDAdmissionControllerInjectConfigMode                = "DD_ADMISSION_CONTROLLER_INJECT_CONFIG_MODE"
	DDAdmissionControllerInjectTags                      = "DD_ADMISSION_CONTROLLER_INJECT_TAGS_ENABLED"
	DDAdmissionControllerLocalServiceName                = "DD_ADMISSION_CONTROLLER_INJECT_CONFIG_LOCAL_SERVICE_NAME"
	DDAdmissionControllerMutateUnlabelled                = "DD_ADMISSION_CONTROLLER_MUTATE_UNLABELLED"
	DDAdmissionControllerServiceName                     = "DD_ADMISSION_CONTROLLER_SERVICE_NAME"
	DDAdmissionControllerFailurePolicy                   = "DD_ADMISSION_CONTROLLER_FAILURE_POLICY"
	DDAdmissionControllerWebhookName                     = "DD_ADMISSION_CONTROLLER_WEBHOOK_NAME"
	DDAdmissionControllerRegistryName                    = "DD_ADMISSION_CONTROLLER_CONTAINER_REGISTRY"
	DDAdmissionControllerCWSInstrumentationEnabled       = "DD_ADMISSION_CONTROLLER_CWS_INSTRUMENTATION_ENABLED"
	DDAdmissionControllerCWSInstrumentationMode          = "DD_ADMISSION_CONTROLLER_CWS_INSTRUMENTATION_MODE"
)
