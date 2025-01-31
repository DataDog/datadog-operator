// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package admissioncontroller

const (
	admissionControllerPortName                = "admissioncontrollerport"
	admissionControllerSocketCommunicationMode = "socket"

	// DefaultAdmissionControllerServicePort default admission controller service port
	defaultAdmissionControllerServicePort = 443
	// DefaultAdmissionControllerTargetPort default admission controller pod port
	defaultAdmissionControllerTargetPort = 8000
	// DefaultAdmissionControllerWebhookName default admission controller webhook name
	defaultAdmissionControllerWebhookName string = "datadog-webhook"
)
