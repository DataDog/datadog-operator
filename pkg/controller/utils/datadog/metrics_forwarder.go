// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package datadog

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/pkg/apis/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/pkg/controller/utils"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/condition"
	"github.com/go-logr/logr"
	api "github.com/zorkian/go-datadog-api"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	defaultMetricsRetryInterval = 15 * time.Second
	defaultSendMetricsInterval  = 15 * time.Second
	defaultMetricsNamespace     = "datadog.operator"
	gaugeType                   = "gauge"
	deploymentSuccessValue      = 1.0
	deploymentFailureValue      = 0.0
	deploymentMetricFormat      = "%s.%s.deployment.success"
	stateTagFormat              = "state:%s"
	clusterNameTagFormat        = "cluster_name:%s"
	crNsTagFormat               = "cr_namespace:%s"
	crNameTagFormat             = "cr_name:%s"
	agentName                   = "agent"
	agentEventName              = "Agent"
	clusteragentName            = "clusteragent"
	clusteragentEventName       = "Cluster Agent"
	clustercheckrunnerName      = "clustercheckrunner"
	clustercheckrunnerEventName = "Cluster Check Runner"
)

var (
	// ErrEmptyAPIKey empty APIKey error
	ErrEmptyAPIKey = errors.New("empty api key")
	// ErrEmptyAPPKey empty APPKey error
	ErrEmptyAPPKey = errors.New("empty app key")
)

var (
	datadogOperatorSourceType  = "datadog_operator"
	deploymentCreatedEventType = "created"
	deploymentDeletedEventType = "deleted"
)

// metricsForwarder sends metrics directly to Datadog using the public API
// its lifecycle must be handled by a ForwardersManager
type metricsForwarder struct {
	id                  string
	datadogClient       api.Client
	k8sClient           client.Client
	keysHash            uint64
	retryInterval       time.Duration
	sendMetricsInterval time.Duration
	metricsPrefix       string
	globalTags          []string
	tags                []string
	componentEnabled    map[string]bool
	stopChan            chan struct{}
	namespacedName      types.NamespacedName
	logger              logr.Logger
	delegator           delegatedAPI
}

// delegatedAPI is used for testing purpose, it serves for mocking the Datadog API
type delegatedAPI interface {
	delegatedSendDeploymentMetric(float64, string, []string) error
	delegatedSendDeploymentEvent(string, string) error
	delegatedValidateCreds(string, string) (*api.Client, error)
}

// newMetricsForwarder returs a new Datadog MetricsForwarder instance
func newMetricsForwarder(k8sClient client.Client, namespacedName types.NamespacedName) *metricsForwarder {
	return &metricsForwarder{
		id:                  namespacedName.String(),
		k8sClient:           k8sClient,
		namespacedName:      namespacedName,
		retryInterval:       defaultMetricsRetryInterval,
		sendMetricsInterval: defaultSendMetricsInterval,
		metricsPrefix:       defaultMetricsNamespace,
		stopChan:            make(chan struct{}),
		logger:              log.WithValues("CustomResource.Namespace", namespacedName.Namespace, "CustomResource.Name", namespacedName.Name),
		componentEnabled: map[string]bool{
			agentName:              false,
			clusteragentName:       false,
			clustercheckrunnerName: false,
		},
	}
}

// start establishes a connection with the Datadog API
// it starts sending deployment metrics once the connection is validated
// designed to run a separate goroutine and stopped using the stop method
func (mf *metricsForwarder) start() {
	mf.logger.Info("Starting Datadog metrics forwarder")

	// Global tags need to be set only once
	mf.initGlobalTags()

	// wait.PollUntil is blocking until mf.connectToDatadogAPI returns true or stopChan is closed
	// wait.PollUntil keeps retrying to connect to the Datadog API without returning an error
	// wait.PollUntil returns an error only when stopChan is closed
	if err := wait.PollUntil(mf.retryInterval, mf.connectToDatadogAPI, mf.stopChan); err == wait.ErrWaitTimeout {
		// stopChan was closed while trying to connect to Datadog API
		// The metrics forwarder stopped by the ForwardersManager
		mf.logger.Info("Shutting down Datadog metrics forwarder")
		return
	}

	mf.logger.Info("Datadog metrics forwarder initilized successfully")

	if err := mf.sendStartupEvent(); err != nil {
		mf.logger.Error(err, "cannot send operator startup event")
	}

	metricsTicker := time.NewTicker(mf.sendMetricsInterval)
	defer metricsTicker.Stop()
	for {
		select {
		case <-mf.stopChan:
			// The metrics forwarder stopped by the ForwardersManager
			mf.logger.Info("Shutting down Datadog metrics forwarder")
			return
		case <-metricsTicker.C:
			if err := mf.forwardMetrics(); err != nil {
				if apierrors.IsConflict(err) {
					mf.logger.Info("Unable to update CR status due to update conflict")
				} else {
					mf.logger.Error(err, "an error occured while sending metrics or updating CR status")
				}
			}
		}
	}
}

// stop closes the stopChan to stop the start method
func (mf *metricsForwarder) stop() {
	close(mf.stopChan)
}

// connectToDatadogAPI ensures the connection to the Datadog API is valid
// implements wait.ConditionFunc and never returns error to keep retrying
func (mf *metricsForwarder) connectToDatadogAPI() (bool, error) {
	dad, err := mf.getDatadogAgentDeployment()
	if err != nil {
		mf.logger.Error(err, "cannot get DatadogAgentDeployment to get Datadog credentials,  will retry later...")
		return false, nil
	}
	mf.logger.Info("Getting Datadog credentials")
	apiKey, appKey, err := mf.getCredentials(dad)
	if err != nil {
		mf.logger.Error(err, "cannot get Datadog credentials,  will retry later...")
		if err = mf.updateStatusIfNeeded(dad, err); err != nil {
			mf.logger.Error(err, "cannot update DatadogAgentDeployment status")
		}
		return false, nil
	}
	mf.logger.Info("Initializing Datadog metrics forwarder")
	if err := mf.initAPIClient(apiKey, appKey); err != nil {
		mf.logger.Error(err, "cannot get Datadog metrics forwarder to send deployment metrics, will retry later...")
		if err = mf.updateStatusIfNeeded(dad, err); err != nil {
			mf.logger.Error(err, "cannot update DatadogAgentDeployment status")
		}
		return false, nil
	}
	if err = mf.updateStatusIfNeeded(dad, nil); err != nil {
		mf.logger.Error(err, "cannot update DatadogAgentDeployment status")
	}
	return true, nil
}

// forwardMetrics sends metrics to Datadog
// it tries to refresh credentials each time it's called
// forwardMetrics updates status conditions of the Custom Resource
// related to Datadog metrics forwarding by calling updateStatusIfNeeded
func (mf *metricsForwarder) forwardMetrics() error {
	dad, err := mf.getDatadogAgentDeployment()
	if err != nil {
		mf.logger.Error(err, "cannot get DatadogAgentDeployment to get deployment metrics")
		return err
	}
	apiKey, appKey, err := mf.getCredentials(dad)
	if err != nil {
		mf.logger.Error(err, "cannot get Datadog credentials")
		if err = mf.updateStatusIfNeeded(dad, err); err != nil {
			mf.logger.Error(err, "cannot update DatadogAgentDeployment status")
		}
		return err
	}
	if err := mf.updateCredsIfNeeded(apiKey, appKey); err != nil {
		mf.logger.Error(err, "cannot update Datadog credentials")
		if err = mf.updateStatusIfNeeded(dad, err); err != nil {
			mf.logger.Error(err, "cannot update DatadogAgentDeployment status")
		}
		return err
	}
	mf.logger.Info("Collecting metrics and events")
	mf.updateTags(dad)
	status := dad.Status.DeepCopy()
	if err := mf.sendStatusMetrics(status); err != nil {
		mf.logger.Error(err, "cannot send status metrics to Datadog")
		if err = mf.updateStatusIfNeeded(dad, err); err != nil {
			mf.logger.Error(err, "cannot update DatadogAgentDeployment status")
		}
		return err
	}
	if err := mf.sendStatusEvents(status); err != nil {
		mf.logger.Error(err, "cannot send events to Datadog")
		if err = mf.updateStatusIfNeeded(dad, err); err != nil {
			mf.logger.Error(err, "cannot update DatadogAgentDeployment status")
		}
		return err
	}
	return mf.updateStatusIfNeeded(dad, err)
}

// sendStartupEvent posts the operator startup event
// it must be called only once, when the operator starts
func (mf *metricsForwarder) sendStartupEvent() error {
	event := &api.Event{
		Time:       api.Int(int(time.Now().Unix())),
		Title:      api.String(fmt.Sprintf("New Datadog Operator Custom Resource - %s", mf.id)),
		EventType:  api.String("Custom Resource Creation"),
		SourceType: api.String(datadogOperatorSourceType),
		Tags:       mf.globalTags,
	}
	if _, err := mf.datadogClient.PostEvent(event); err != nil {
		return err
	}
	return nil
}

// initAPIClient initializes and validates the Datadog API client
func (mf *metricsForwarder) initAPIClient(apiKey, appKey string) error {
	if mf.delegator == nil {
		mf.delegator = mf
	}
	datadogClient, err := mf.validateCreds(apiKey, appKey)
	if err != nil {
		return err
	}
	mf.datadogClient = *datadogClient
	mf.keysHash = hashKeys(apiKey, appKey)
	return nil
}

// updateCredsIfNeeded used to update Datadog apiKey and appKey if they change
func (mf *metricsForwarder) updateCredsIfNeeded(apiKey, appKey string) error {
	if mf.keysHash != hashKeys(apiKey, appKey) {
		return mf.initAPIClient(apiKey, appKey)
	}
	return nil
}

// validateCreds returns validates the creds by querying the Datadog API
func (mf *metricsForwarder) validateCreds(apiKey, appKey string) (*api.Client, error) {
	return mf.delegator.delegatedValidateCreds(apiKey, appKey)
}

// delegatedValidateCreds is separated from validateCreds to facilitate mocking the Datadog API
func (mf *metricsForwarder) delegatedValidateCreds(apiKey, appKey string) (*api.Client, error) {
	datadogClient := api.NewClient(apiKey, appKey)
	valid, err := datadogClient.Validate()
	if err != nil {
		return nil, fmt.Errorf("cannot validate datadog credentials: %v", err)
	}
	if !valid {
		return nil, errors.New("invalid datadog credentials")
	}
	return datadogClient, nil
}

// sendStatusMetrics forwards metrics for each component deployment (agent, clusteragent, clustercheck runner)
// based on the status of DatadogAgentDeployment
func (mf *metricsForwarder) sendStatusMetrics(status *datadoghqv1alpha1.DatadogAgentDeploymentStatus) error {
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
		tags := mf.tagsWithState(string(status.Agent.State))
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
		tags := mf.tagsWithState(string(status.ClusterAgent.State))
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
		tags := mf.tagsWithState(string(status.ClusterChecksRunner.State))
		if err := mf.sendDeploymentMetric(metricValue, clustercheckrunnerName, tags); err != nil {
			return err
		}
	}

	return nil
}

// tagsWithState used to append the state tag
func (mf *metricsForwarder) tagsWithState(state string) []string {
	return append(mf.globalTags, append(mf.tags, fmt.Sprintf(stateTagFormat, state))...)
}

// sendStatusEvents forwards events for each component deployment (agent, clusteragent, clustercheck runner)
// based on the status of DatadogAgentDeployment
func (mf *metricsForwarder) sendStatusEvents(status *datadoghqv1alpha1.DatadogAgentDeploymentStatus) error {
	if status == nil {
		return errors.New("nil status")
	}

	// Agent deployment events
	if eventType := mf.getDeploymentEventType(status.Agent == nil, agentName); eventType != nil {
		if err := mf.sendDeploymentEvent(agentEventName, *eventType); err != nil {
			return err
		}
	}

	// Cluster Agent deployment events
	if eventType := mf.getDeploymentEventType(status.ClusterAgent == nil, clusteragentName); eventType != nil {
		if err := mf.sendDeploymentEvent(clusteragentEventName, *eventType); err != nil {
			return err
		}
	}

	// Cluster Check Runner deployment events
	if eventType := mf.getDeploymentEventType(status.ClusterChecksRunner == nil, clustercheckrunnerName); eventType != nil {
		if err := mf.sendDeploymentEvent(clustercheckrunnerEventName, *eventType); err != nil {
			return err
		}
	}

	return nil
}

// getDeploymentEventType retrieves the deployment event type based on the status and the last recorded state of a component
// it could be shared between the different components (agent, cluster agent, cluster check runner)
// it returns nil if no event has been detected
func (mf *metricsForwarder) getDeploymentEventType(isComponentNil bool, componentName string) *string {
	if !isComponentNil && !mf.componentEnabled[componentName] {
		// The component is enabled (not nil) and wasn't enabled before
		// Creation event detected
		mf.componentEnabled[componentName] = true
		return &deploymentCreatedEventType
	}
	if isComponentNil && mf.componentEnabled[componentName] {
		// The component is disabled (nil) and was enabled before
		// Deletion event detected
		mf.componentEnabled[componentName] = false
		return &deploymentDeletedEventType
	}
	return nil
}

// sendDeploymentMetric is a generic method used to forward component deployment metrics to Datadog
func (mf *metricsForwarder) sendDeploymentMetric(metricValue float64, component string, tags []string) error {
	return mf.delegator.delegatedSendDeploymentMetric(metricValue, component, tags)
}

// sendDeploymentEvent is a generic method used to forward component deployment events to Datadog
func (mf *metricsForwarder) sendDeploymentEvent(component, eventType string) error {
	return mf.delegator.delegatedSendDeploymentEvent(component, eventType)
}

// delegatedSendDeploymentMetric is separated from sendDeploymentMetric to facilitate mocking the Datadog API
func (mf *metricsForwarder) delegatedSendDeploymentMetric(metricValue float64, component string, tags []string) error {
	ts := float64(time.Now().Unix())
	metricName := fmt.Sprintf(deploymentMetricFormat, mf.metricsPrefix, component)
	serie := []api.Metric{
		{
			Metric: api.String(metricName),
			Points: []api.DataPoint{
				{
					api.Float64(ts),
					api.Float64(metricValue),
				},
			},
			Type: api.String(gaugeType),
			Tags: tags,
		},
	}
	return mf.datadogClient.PostMetrics(serie)
}

// delegatedSendDeploymentEvent is separated from sendDeploymentEvent to facilitate mocking the Datadog API
func (mf *metricsForwarder) delegatedSendDeploymentEvent(component, eventType string) error {
	event := &api.Event{
		Time:       api.Int(int(time.Now().Unix())),
		Title:      api.String(fmt.Sprintf("%s deployment %s", component, eventType)),
		EventType:  api.String(eventType),
		SourceType: api.String(datadogOperatorSourceType),
		Tags:       append(mf.globalTags, mf.tags...),
	}
	if _, err := mf.datadogClient.PostEvent(event); err != nil {
		return err
	}
	return nil
}

// updateTags updates tags of the DatadogAgentDeployment
func (mf *metricsForwarder) updateTags(dad *datadoghqv1alpha1.DatadogAgentDeployment) {
	if dad == nil {
		mf.tags = []string{}
		return
	}
	tags := []string{}
	if dad.Spec.ClusterName != "" {
		tags = append(tags, fmt.Sprintf(clusterNameTagFormat, dad.Spec.ClusterName))
	}
	for labelKey, labelValue := range dad.GetLabels() {
		tags = append(tags, fmt.Sprintf("%s:%s", labelKey, labelValue))
	}
	mf.tags = tags
}

// initGlobalTags defines the Custom Resource namespace and name tags
func (mf *metricsForwarder) initGlobalTags() {
	mf.globalTags = append(mf.globalTags, []string{
		fmt.Sprintf(crNsTagFormat, mf.namespacedName.Namespace),
		fmt.Sprintf(crNameTagFormat, mf.namespacedName.Name),
	}...)
}

// hashKeys is used to detect if credentials have changed
// hashKeys is NOT a security function
func hashKeys(apiKey, appKey string) uint64 {
	h := fnv.New64()
	_, _ = h.Write([]byte(apiKey))
	_, _ = h.Write([]byte(appKey))
	return h.Sum64()
}

// getDatadogAgentDeployment retrieves the DatadogAgentDeployment using Get client method
func (mf *metricsForwarder) getDatadogAgentDeployment() (*datadoghqv1alpha1.DatadogAgentDeployment, error) {
	dad := &datadoghqv1alpha1.DatadogAgentDeployment{}
	err := mf.k8sClient.Get(context.TODO(), mf.namespacedName, dad)
	return dad, err
}

// getCredentials returns the Datadog API Key and APP Key, it returns an error if one key is missing
func (mf *metricsForwarder) getCredentials(dad *datadoghqv1alpha1.DatadogAgentDeployment) (string, string, error) {
	var err error
	apiKey, appKey := "", ""
	if dad.Spec.Credentials.APIKey != "" {
		apiKey = dad.Spec.Credentials.APIKey
	} else {
		apiKey, err = mf.getKeyFromSecret(dad, utils.GetAPIKeySecretName, datadoghqv1alpha1.DefaultAPIKeyKey)
		if err != nil {
			return "", "", err
		}
	}
	if dad.Spec.Credentials.AppKey != "" {
		appKey = dad.Spec.Credentials.AppKey
	} else {
		appKey, err = mf.getKeyFromSecret(dad, utils.GetAppKeySecretName, datadoghqv1alpha1.DefaultAPPKeyKey)
		if err != nil {
			return "", "", err
		}
	}

	if apiKey == "" {
		return "", "", ErrEmptyAPIKey
	}

	if appKey == "" {
		return "", "", ErrEmptyAPPKey
	}

	return apiKey, appKey, nil
}

// getKeyFromSecret used to retrieve an api or app key from a secret object
func (mf *metricsForwarder) getKeyFromSecret(dad *datadoghqv1alpha1.DatadogAgentDeployment, nameFunc func(*datadoghqv1alpha1.DatadogAgentDeployment) string, dataKey string) (string, error) {
	secretName := nameFunc(dad)
	secret := &corev1.Secret{}
	err := mf.k8sClient.Get(context.TODO(), types.NamespacedName{Namespace: dad.Namespace, Name: secretName}, secret)
	if err != nil {
		return "", err
	}
	return string(secret.Data[dataKey]), nil
}

// updateStatusIfNeeded updates the Datadog metrics forwarding status in the DatadogAgentDeployment status
func (mf *metricsForwarder) updateStatusIfNeeded(dad *datadoghqv1alpha1.DatadogAgentDeployment, err error) error {
	now := metav1.NewTime(time.Now())
	newStatus := dad.Status.DeepCopy()
	condition.UpdateDatadogAgentDeploymentStatusConditionsFailure(newStatus, now, datadoghqv1alpha1.ConditionTypeDatadogMetricsError, err)
	if err == nil {
		condition.UpdateDatadogAgentDeploymentStatusCondition(newStatus, now, datadoghqv1alpha1.ConditionTypeActiveDatadogMetrics, corev1.ConditionTrue, "Datadog metrics and events forwarding ok", false)
	} else {
		condition.UpdateDatadogAgentDeploymentStatusCondition(newStatus, now, datadoghqv1alpha1.ConditionTypeActiveDatadogMetrics, corev1.ConditionFalse, "Datadog metrics and events forwarding error", false)
	}
	if !apiequality.Semantic.DeepEqual(&dad.Status, newStatus) {
		updatedDad := dad.DeepCopy()
		updatedDad.Status = *newStatus
		return mf.k8sClient.Status().Update(context.TODO(), updatedDad)
	}
	return nil
}
