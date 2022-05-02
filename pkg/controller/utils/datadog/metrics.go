// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadog

import (
	"errors"
	"fmt"
	"reflect"
	"strings"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

const (
	agentName                  = "agent"
	clusteragentName           = "clusteragent"
	clustercheckrunnerName     = "clustercheckrunner"
	deploymentSuccessValue     = 1.0
	deploymentFailureValue     = 0.0
	deploymentMetricFormat     = "%s.deployment.success"
	kindDatadogAgent           = "DatadogAgent"
	kindDatadogMonitor         = "DatadogMonitor"
	monitorSuccessValue        = 1.0
	monitorFailureValue        = 0.0
	monitorMetricFormat        = "monitor.%s"
	monitorIDTagFormat         = "monitor_id:%d"
	monitorStateTagFormat      = "monitor_state:%s"
	monitorSyncStatusTagFormat = "monitor_sync_status:%s"
	reconcileSuccessValue      = 1.0
	reconcileFailureValue      = 0.0
	reconcileErrTagFormat      = "reconcile_err:%s"
	reconcileMetricFormat      = "reconcile.success"
	stateTagFormat             = "state:%s"
)

// Entryway for processing the metrics for the specified object of the metricsForwarder,
// will route accordingly based on the objectKind and namespacedName reference
func (mf *metricsForwarder) processMetrics() error {
	switch mf.objectKind {
	case kindDatadogMonitor:
		mf.logger.V(1).Info("Handling DatadogMonitor Metrics")
		ddm, err := mf.getDatadogMonitor()
		if err != nil {
			mf.logger.Error(err, "Error getting DatadogMonitor for metric collection")
			return err
		}

		if err := mf.sendMonitorStatus(&ddm.Status); err != nil {
			mf.logger.Error(err, "Error collecting metrics for DatdogMonitor status")
			return err
		}
	case kindDatadogAgent:
		mf.logger.Info("Handling DatadogAgent Metrics")
		dda, err := mf.getDatadogAgent()
		if err != nil {
			mf.logger.Error(err, "Error getting DatadogAgent for metric collection")
			return err
		}
		mf.updateTags(dda)

		status := dda.Status.DeepCopy()
		if err := mf.sendStatusMetrics(status); err != nil {
			mf.logger.Error(err, "Error collecting metrics for DatadogAgent status")
			return err
		}
	default:
		return errors.New("can't process metrics, Metrics Forwarder has no set Kind")
	}

	// Report the status of the reconciler at the same periodic interal
	return mf.processReconcileMetric()
}

// DatadogAgent Functions

// sendStatusMetrics forwards metrics for each component deployment (agent, clusteragent, clustercheck runner)
// based on the status of DatadogAgent
func (mf *metricsForwarder) sendStatusMetrics(status *datadoghqv1alpha1.DatadogAgentStatus) error {
	if status == nil {
		return errors.New("nil status")
	}
	var metricValue float64

	// Agent deployment metrics
	if status.Agent != nil {
		if status.Agent.Available == status.Agent.Desired {
			metricValue = deploymentSuccessValue
		} else {
			metricValue = deploymentFailureValue
		}
		tags := mf.tagsWithExtraTag(stateTagFormat, status.Agent.State)
		if err := mf.sendDeploymentMetric(metricValue, agentName, tags); err != nil {
			return err
		}
	}

	// Cluster Agent deployment metrics
	if status.ClusterAgent != nil {
		if status.ClusterAgent.AvailableReplicas == status.ClusterAgent.Replicas {
			metricValue = deploymentSuccessValue
		} else {
			metricValue = deploymentFailureValue
		}
		tags := mf.tagsWithExtraTag(stateTagFormat, status.ClusterAgent.State)
		if err := mf.sendDeploymentMetric(metricValue, clusteragentName, tags); err != nil {
			return err
		}
	}

	// Cluster Check Runner deployment metrics
	if status.ClusterChecksRunner != nil {
		if status.ClusterChecksRunner.AvailableReplicas == status.ClusterChecksRunner.Replicas {
			metricValue = deploymentSuccessValue
		} else {
			metricValue = deploymentFailureValue
		}
		tags := mf.tagsWithExtraTag(stateTagFormat, status.ClusterChecksRunner.State)
		if err := mf.sendDeploymentMetric(metricValue, clustercheckrunnerName, tags); err != nil {
			return err
		}
	}

	return nil
}

// sendDeploymentMetric is a generic method used to forward component deployment metrics to Datadog
func (mf *metricsForwarder) sendDeploymentMetric(metricValue float64, component string, tags []string) error {
	return mf.delegator.delegatedSendDeploymentMetric(metricValue, component, tags)
}

// delegatedSendDeploymentMetric is separated from sendDeploymentMetric to facilitate mocking the Datadog API
func (mf *metricsForwarder) delegatedSendDeploymentMetric(metricValue float64, component string, tags []string) error {
	metricName := fmt.Sprintf(deploymentMetricFormat, component)
	return mf.gauge(metricName, metricValue, tags)
}

// DatadogMonitor Functions

// sendMonitorStatus forwards metrics for each condition of the specified DatadogMonitor, tagging
// with relevant monitor tags when available
func (mf *metricsForwarder) sendMonitorStatus(status *datadoghqv1alpha1.DatadogMonitorStatus) error {
	if status == nil {
		return errors.New("nil status")
	}

	tags := []string{}
	if status.ID != 0 {
		tags = append(tags, fmt.Sprintf(monitorIDTagFormat, status.ID))
	}
	if status.MonitorState != "" {
		tags = append(tags, fmt.Sprintf(monitorStateTagFormat, status.MonitorState))
	}
	if status.SyncStatus != "" {
		tags = append(tags, fmt.Sprintf(monitorSyncStatusTagFormat, status.SyncStatus))
	}

	// Status Condition
	if status.Conditions != nil {
		for _, condition := range status.Conditions {
			metricName := strings.ToLower(string(condition.Type))
			metricValue := monitorFailureValue
			if condition.Status == "True" {
				metricValue = monitorSuccessValue
			}

			if err := mf.sendMonitorMetric(metricValue, metricName, tags); err != nil {
				// Continue through remaining conditions
				mf.logger.Error(err, "Error reporting metric", "metricName", metricName, "metricValue", metricValue, "tags", tags)
			}
		}
	}

	return nil
}

// sendMonitorMetric is a generic method used to forward component monitor metrics to Datadog
func (mf *metricsForwarder) sendMonitorMetric(metricValue float64, component string, tags []string) error {
	return mf.delegator.delegatedSendMonitorMetric(metricValue, component, tags)
}

// delegatedSendMonitorMetric is separated from sendDeploymentMetric to facilitate mocking the Datadog API
func (mf *metricsForwarder) delegatedSendMonitorMetric(metricValue float64, component string, tags []string) error {
	metricName := fmt.Sprintf(monitorMetricFormat, component)
	return mf.gauge(metricName, metricValue, tags)
}

// Gets the last error for the reconciler and sends a metric up. The reconcile errors are sent to the
// metricsForwarder.errorChan channel to handle when they occur, however, this is used to report the metric
// on a periodic basis
func (mf *metricsForwarder) processReconcileMetric() error {
	reconcileErr := mf.getLastReconcileError()
	var metricValue float64
	var tags []string
	metricValue, tags, err := mf.prepareReconcileMetric(reconcileErr)
	if err != nil {
		mf.logger.Error(err, "cannot prepare reconcile metric")
		return err
	}
	if err = mf.sendReconcileMetric(metricValue, tags); err != nil {
		mf.logger.Error(err, "cannot send reconcile errors metric to Datadog")
		return err
	}

	return nil
}

// processReconcileError updates lastReconcileErr
// and sends reconcile metrics based on the reconcile errors
func (mf *metricsForwarder) processReconcileError(reconcileErr error) error {
	if reflect.DeepEqual(mf.getLastReconcileError(), reconcileErr) {
		// Error didn't change
		return nil
	}

	// Update lastReconcileErr with the new reconcile error
	mf.setLastReconcileError(reconcileErr)

	// Prepare and send reconcile metric
	metricValue, tags, err := mf.prepareReconcileMetric(reconcileErr)
	if err != nil {
		return err
	}
	return mf.sendReconcileMetric(metricValue, tags)
}

// prepareReconcileMetric returns the corresponding metric value and tags for the last reconcile error metric
// returns an error if lastReconcileErr still equals the init value
func (mf *metricsForwarder) prepareReconcileMetric(reconcileErr error) (float64, []string, error) {
	var metricValue float64
	var tags []string

	if errors.Is(reconcileErr, errInitValue) {
		// Metrics forwarder didn't receive any reconcile error
		// lastReconcileErr has never been updated
		return metricValue, nil, errors.New("last reconcile error not updated")
	}

	if reconcileErr == nil {
		metricValue = reconcileSuccessValue
		tags = mf.tagsWithExtraTag(reconcileErrTagFormat, "null")
	} else {
		metricValue = reconcileFailureValue
		reason := string(apierrors.ReasonForError(reconcileErr))
		if reason == "" {
			reason = reconcileErr.Error()
		}
		tags = mf.tagsWithExtraTag(reconcileErrTagFormat, reason)
	}
	return metricValue, tags, nil
}

// getLastReconcileError provides thread-safe read access to lastReconcileErr
func (mf *metricsForwarder) getLastReconcileError() error {
	mf.Lock()
	defer mf.Unlock()
	return mf.lastReconcileErr
}

// setLastReconcileError provides thread-safe write access to lastReconcileErr
func (mf *metricsForwarder) setLastReconcileError(newErr error) {
	mf.Lock()
	defer mf.Unlock()
	mf.lastReconcileErr = newErr
}

// sendReconcileMetric is used to forward reconcile metrics to Datadog
func (mf *metricsForwarder) sendReconcileMetric(metricValue float64, tags []string) error {
	return mf.delegator.delegatedSendReconcileMetric(metricValue, tags)
}

// delegatedSendReconcileMetric is separated from sendReconcileMetric to facilitate mocking the Datadog API
func (mf *metricsForwarder) delegatedSendReconcileMetric(metricValue float64, tags []string) error {
	return mf.gauge(reconcileMetricFormat, metricValue, tags)
}
