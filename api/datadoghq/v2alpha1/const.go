// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package v2alpha1

// This file tracks constants related to the DatadogAgent CRD

const (
	// DefaultAPPKeyKey default app-key key (use in secret for instance).
	DefaultAPPKeyKey = "app_key"
	// DefaultAPIKeyKey default api-key key (use in secret for instance).
	DefaultAPIKeyKey = "api_key"
)

// Experiment signal annotations. The fleet daemon writes these annotations to
// request state transitions; the reconciler clears them after processing.
const (
	// AnnotationExperimentID is the annotation key for the experiment signal ID.
	AnnotationExperimentID = "experiment.datadoghq.com/id"
	// AnnotationExperimentSignal is the annotation key for the experiment signal type.
	AnnotationExperimentSignal = "experiment.datadoghq.com/signal"
)

// Fleet pending-operation annotations. The fleet daemon writes these
// annotations to durably track the single in-flight async operation for a DDA
// so it can recover after restarts.
const (
	// AnnotationPendingTaskID is the RC updater task ID currently awaiting completion.
	AnnotationPendingTaskID = "fleet.datadoghq.com/pending-task-id"
	// AnnotationPendingAction is the pending fleet action, such as start/stop/promote.
	AnnotationPendingAction = "fleet.datadoghq.com/pending-action"
	// AnnotationPendingExperimentID is the stable experiment identity (`params.version`).
	AnnotationPendingExperimentID = "fleet.datadoghq.com/pending-experiment-id"
	// AnnotationPendingPackage is the RC package whose Task.State/config versions should be updated.
	AnnotationPendingPackage = "fleet.datadoghq.com/pending-package"
	// AnnotationPendingResultVersion is the RC config version to write on
	// success when it differs from the experiment identity, such as promote.
	AnnotationPendingResultVersion = "fleet.datadoghq.com/pending-result-version"
)
