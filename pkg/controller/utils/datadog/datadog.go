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
	api "github.com/zorkian/go-datadog-api"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

const (
	defaultSendMetricsInterval  = 15 * time.Second
	defaultMetricsNamespace     = "datadog.operator"
	gaugeType                   = "gauge"
	deploymentSuccessValue      = 1.0
	deploymentFailureValue      = 0.0
	deploymentMetricFormat      = "%s.%s.deploymentsuccess"
	stateTagFormat              = "state:%s"
	clusterNameTagFormat        = "cluster_name:%s"
	agentName                   = "agent"
	agentEventName              = "Agent"
	clusteragentName            = "clusteragent"
	clusteragentEventName       = "Cluster Agent"
	clustercheckrunnerName      = "clustercheckrunner"
	clustercheckrunnerEventName = "Cluster Check Runner"
	deploymentCreatedEventType  = "created"
	deploymentDeletedEventType  = "deleted"
)

var (
	// ErrEmptyAPIKey empty APIKey error
	ErrEmptyAPIKey = errors.New("empty api key")
	// ErrEmptyAPPKey empty APPKey error
	ErrEmptyAPPKey = errors.New("empty app key")
)

var log = logf.Log.WithName("DatadogMetricsForwarder")

// MetricsForwarder sends metrics directly to Datadog using the public API
type MetricsForwarder struct {
	datadogClient       api.Client
	k8sClient           client.Client
	keysHash            uint64
	retryInterval       time.Duration
	sendMetricsInterval time.Duration
	metricsPrefix       string
	tags                []string
	componentEnabled    map[string]bool
	delegator           delegatedAPI
}

// delegatedAPI is used for testing purpose, it serves for mocking the Datadog API
type delegatedAPI interface {
	delegatedSendDeploymentMetric(float64, string, []string) error
	delegatedSendDeploymentEvent(string, string) error
	delegatedValidateCreds(string, string) (*api.Client, error)
}

// NewMetricsForwarder returs a new Datadog MetricsForwarder instance
// MetricsForwarder implements the controller-runtime Runnable interface
func NewMetricsForwarder(k8sClient client.Client, retryInterval time.Duration) *MetricsForwarder {
	return &MetricsForwarder{
		k8sClient:           k8sClient,
		retryInterval:       retryInterval,
		sendMetricsInterval: defaultSendMetricsInterval,
		metricsPrefix:       defaultMetricsNamespace,
		componentEnabled: map[string]bool{
			agentName:              false,
			clusteragentName:       false,
			clustercheckrunnerName: false,
		},
	}
}

// Start establishes a connection with the Datadog API
// it starts sending deployment metrics once the connection is validated
func (dd *MetricsForwarder) Start(stop <-chan struct{}) error {
	// wait.PollInfinite is blocking until dd.connectToDatadogAPI returns true
	if err := wait.PollInfinite(dd.retryInterval, dd.connectToDatadogAPI); err == nil {
		log.Info("Datadog metrics forwarder initilized successfully")
	}

	if err := dd.sendStartupEvent(); err != nil {
		log.Error(err, "cannot send operator startup event")
	}

	metricsTicker := time.NewTicker(dd.sendMetricsInterval)
	defer metricsTicker.Stop()
	for {
		select {
		case <-stop:
			return nil
		case <-metricsTicker.C:
			if err := dd.forwardMetrics(); err != nil {
				log.Error(err, "cannot forward metrics to Datadog")
			}
		}
	}
}

// connectToDatadogAPI ensures the connection to the Datadog API is valid
// implements wait.ConditionFunc and never returns error to keep retrying
func (dd *MetricsForwarder) connectToDatadogAPI() (bool, error) {
	dad, err := dd.listDatadogAgentDeployment()
	if err != nil {
		log.Error(err, "cannot list DatadogAgentDeploymentList to get Datadog credentials")
		return false, nil
	}
	log.Info("Getting Datadog credentials")
	apiKey, appKey, err := dd.getCredentials(dad)
	if err != nil {
		log.Error(err, "cannot get Datadog credentials")
		return false, nil
	}
	log.Info("Initializing Datadog metrics forwarder")
	if err := dd.initAPIClient(apiKey, appKey); err != nil {
		log.Error(err, "cannot get Datadog metrics forwarder to send deployment metrics, will retry later...")
		return false, nil
	}
	return true, nil
}

// forwardMetrics sends metrics to Datadog
// it tries to refresh credentials each time it's called
func (dd *MetricsForwarder) forwardMetrics() error {
	dad, err := dd.listDatadogAgentDeployment()
	if err != nil {
		log.Error(err, "cannot list DatadogAgentDeploymentList to get deployment metrics")
		return err
	}
	apiKey, appKey, err := dd.getCredentials(dad)
	if err != nil {
		log.Error(err, "cannot get Datadog credentials")
		return err
	}
	if err := dd.updateCredsIfNeeded(apiKey, appKey); err != nil {
		log.Error(err, "cannot update Datadog credentials")
		return err
	}
	log.Info("Collecting metrics and events")
	dd.setTags(dad)
	status := dad.Status.DeepCopy()
	if err := dd.sendStatusMetrics(status); err != nil {
		log.Error(err, "cannot send status metrics to Datadog")
		return err
	}
	if err := dd.sendStatusEvents(status); err != nil {
		log.Error(err, "cannot send events to Datadog")
		return err
	}
	return nil
}

// sendStartupEvent posts the operator startup event
// it must be called only once, when the operator starts
func (dd *MetricsForwarder) sendStartupEvent() error {
	event := &api.Event{
		Time:      api.Int(int(time.Now().Unix())),
		Title:     api.String("Datadog Operator has started"),
		EventType: api.String("Datadog Operator Startup"),
	}
	if _, err := dd.datadogClient.PostEvent(event); err != nil {
		return err
	}
	return nil
}

// initAPIClient initializes and validates the Datadog API client
func (dd *MetricsForwarder) initAPIClient(apiKey, appKey string) error {
	if dd.delegator == nil {
		dd.delegator = dd
	}
	datadogClient, err := dd.validateCreds(apiKey, appKey)
	if err != nil {
		return err
	}
	dd.datadogClient = *datadogClient
	dd.keysHash = hashKeys(apiKey, appKey)
	return nil
}

// updateCredsIfNeeded used to update Datadog apiKey and appKey if they change
func (dd *MetricsForwarder) updateCredsIfNeeded(apiKey, appKey string) error {
	if dd.keysHash != hashKeys(apiKey, appKey) {
		return dd.initAPIClient(apiKey, appKey)
	}
	return nil
}

// validateCreds returns validates the creds by querying the Datadog API
func (dd *MetricsForwarder) validateCreds(apiKey, appKey string) (*api.Client, error) {
	return dd.delegator.delegatedValidateCreds(apiKey, appKey)
}

// delegatedValidateCreds is separated from validateCreds to facilitate mocking the Datadog API
func (dd *MetricsForwarder) delegatedValidateCreds(apiKey, appKey string) (*api.Client, error) {
	datadogClient := api.NewClient(apiKey, appKey)
	valid, err := datadogClient.Validate()
	if !valid || err != nil {
		return nil, fmt.Errorf("datadog client is invalid: %v", err)
	}
	return datadogClient, nil
}

// sendStatusMetrics forwards metrics for each component deployment (agent, clusteragent, clustercheck runner)
// based on the status of DatadogAgentDeployment
func (dd *MetricsForwarder) sendStatusMetrics(status *datadoghqv1alpha1.DatadogAgentDeploymentStatus) error {
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
		tags := append(dd.tags, fmt.Sprintf(stateTagFormat, string(status.Agent.State)))
		if err := dd.sendDeploymentMetric(metricValue, agentName, tags); err != nil {
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
		tags := append(dd.tags, fmt.Sprintf(stateTagFormat, string(status.ClusterAgent.State)))
		if err := dd.sendDeploymentMetric(metricValue, clusteragentName, tags); err != nil {
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
		tags := append(dd.tags, fmt.Sprintf(stateTagFormat, string(status.ClusterChecksRunner.State)))
		if err := dd.sendDeploymentMetric(metricValue, clustercheckrunnerName, tags); err != nil {
			return err
		}
	}

	return nil
}

// sendStatusEvents forwards events for each component deployment (agent, clusteragent, clustercheck runner)
// based on the status of DatadogAgentDeployment
func (dd *MetricsForwarder) sendStatusEvents(status *datadoghqv1alpha1.DatadogAgentDeploymentStatus) error {
	if status == nil {
		return errors.New("nil status")
	}

	// Agent deployment events
	if status.Agent != nil && !dd.componentEnabled[agentName] {
		if err := dd.sendDeploymentEvent(agentEventName, deploymentCreatedEventType); err != nil {
			return err
		}
		dd.componentEnabled[agentName] = true
	} else if status.Agent == nil && dd.componentEnabled[agentName] {
		if err := dd.sendDeploymentEvent(agentEventName, deploymentDeletedEventType); err != nil {
			return err
		}
		dd.componentEnabled[agentName] = false
	}

	// Cluster Agent deployment events
	if status.ClusterAgent != nil && !dd.componentEnabled[clusteragentName] {
		if err := dd.sendDeploymentEvent(clusteragentEventName, deploymentCreatedEventType); err != nil {
			return err
		}
		dd.componentEnabled[clusteragentName] = true
	} else if status.ClusterAgent == nil && dd.componentEnabled[clusteragentName] {
		if err := dd.sendDeploymentEvent(clusteragentEventName, deploymentDeletedEventType); err != nil {
			return err
		}
		dd.componentEnabled[clusteragentName] = false
	}

	// Cluster Check Runner deployment events
	if status.ClusterChecksRunner != nil && !dd.componentEnabled[clustercheckrunnerName] {
		if err := dd.sendDeploymentEvent(clustercheckrunnerEventName, deploymentCreatedEventType); err != nil {
			return err
		}
		dd.componentEnabled[clustercheckrunnerName] = true
	} else if status.ClusterChecksRunner == nil && dd.componentEnabled[clustercheckrunnerName] {
		if err := dd.sendDeploymentEvent(clustercheckrunnerEventName, deploymentDeletedEventType); err != nil {
			return err
		}
		dd.componentEnabled[clustercheckrunnerName] = false
	}
	return nil
}

// sendDeploymentMetric is a generic method used to forward component deployment metrics to Datadog
func (dd *MetricsForwarder) sendDeploymentMetric(metricValue float64, component string, tags []string) error {
	return dd.delegator.delegatedSendDeploymentMetric(metricValue, component, tags)
}

// sendDeploymentEvent is a generic method used to forward component deployment events to Datadog
func (dd *MetricsForwarder) sendDeploymentEvent(component, eventType string) error {
	return dd.delegator.delegatedSendDeploymentEvent(component, eventType)
}

// delegatedSendDeploymentMetric is separated from sendDeploymentMetric to facilitate mocking the Datadog API
func (dd *MetricsForwarder) delegatedSendDeploymentMetric(metricValue float64, component string, tags []string) error {
	ts := float64(time.Now().Unix())
	metricName := fmt.Sprintf(deploymentMetricFormat, dd.metricsPrefix, component)
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
	return dd.datadogClient.PostMetrics(serie)
}

// delegatedSendDeploymentEvent is separated from sendDeploymentEvent to facilitate mocking the Datadog API
func (dd *MetricsForwarder) delegatedSendDeploymentEvent(component, eventType string) error {
	event := &api.Event{
		Time:      api.Int(int(time.Now().Unix())),
		Title:     api.String(fmt.Sprintf("%s deployment %s", component, eventType)),
		EventType: api.String(eventType),
		Tags:      dd.tags,
	}
	if _, err := dd.datadogClient.PostEvent(event); err != nil {
		return err
	}
	return nil
}

// setTags adds tags to the MetricsForwarder from DatadogAgentDeployment
func (dd *MetricsForwarder) setTags(dad *datadoghqv1alpha1.DatadogAgentDeployment) {
	if dad == nil {
		dd.tags = []string{}
		return
	}
	tags := []string{}
	if dad.Spec.ClusterName != "" {
		tags = append(tags, fmt.Sprintf(clusterNameTagFormat, dad.Spec.ClusterName))
	}
	for labelKey, labelValue := range dad.GetLabels() {
		tags = append(tags, fmt.Sprintf("%s:%s", labelKey, labelValue))
	}
	dd.tags = tags
}

// hashKeys is used to detect if credentials have changed
// hashKeys is NOT a security function
func hashKeys(apiKey, appKey string) uint64 {
	h := fnv.New64()
	_, _ = h.Write([]byte(apiKey))
	_, _ = h.Write([]byte(appKey))
	return h.Sum64()
}

// listDatadogAgentDeployment retrieves the DatadogAgentDeployment using List client method
func (dd *MetricsForwarder) listDatadogAgentDeployment() (*datadoghqv1alpha1.DatadogAgentDeployment, error) {
	list := &datadoghqv1alpha1.DatadogAgentDeploymentList{}
	err := dd.k8sClient.List(context.TODO(), list)
	if err != nil {
		return nil, err
	}
	if len(list.Items) == 0 {
		return nil, errors.New("DatadogAgentDeploymentList is empty")
	}
	return &list.Items[0], nil
}

// getCredentials returns the Datadog API Key and APP Key, it returns an error if one key is missing
func (dd *MetricsForwarder) getCredentials(dad *datadoghqv1alpha1.DatadogAgentDeployment) (string, string, error) {
	var err error
	apiKey, appKey := "", ""
	if dad.Spec.Credentials.APIKey != "" {
		apiKey = dad.Spec.Credentials.APIKey
	} else {
		apiKey, err = dd.getKeyFromSecret(dad, utils.GetAPIKeySecretName, datadoghqv1alpha1.DefaultAPIKeyKey)
		if err != nil {
			return "", "", err
		}
	}
	if dad.Spec.Credentials.AppKey != "" {
		appKey = dad.Spec.Credentials.AppKey
	} else {
		appKey, err = dd.getKeyFromSecret(dad, utils.GetAppKeySecretName, datadoghqv1alpha1.DefaultAPPKeyKey)
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
func (dd *MetricsForwarder) getKeyFromSecret(dad *datadoghqv1alpha1.DatadogAgentDeployment, nameFunc func(*datadoghqv1alpha1.DatadogAgentDeployment) string, dataKey string) (string, error) {
	secretName := nameFunc(dad)
	secret := &corev1.Secret{}
	err := dd.k8sClient.Get(context.TODO(), types.NamespacedName{Namespace: dad.Namespace, Name: secretName}, secret)
	if err != nil {
		return "", err
	}
	return string(secret.Data[dataKey]), nil
}
