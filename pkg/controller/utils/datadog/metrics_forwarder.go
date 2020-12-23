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
	"os"
	"reflect"
	"sync"
	"time"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/v1alpha1"
	"github.com/DataDog/datadog-operator/pkg/config"
	"github.com/DataDog/datadog-operator/pkg/controller/utils"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/condition"
	"github.com/DataDog/datadog-operator/pkg/secrets"

	"github.com/go-logr/logr"
	api "github.com/zorkian/go-datadog-api"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
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
	clusteragentName            = "clusteragent"
	clustercheckrunnerName      = "clustercheckrunner"
	reconcileSuccessValue       = 1.0
	reconcileFailureValue       = 0.0
	reconcileMetricFormat       = "%s.reconcile.success"
	reconcileErrTagFormat       = "reconcile_err:%s"
	datadogOperatorSourceType   = "datadog_operator"
	defaultbaseURL              = "https://api.datadoghq.com"
)

var (
	// ErrEmptyAPIKey empty APIKey error
	ErrEmptyAPIKey = errors.New("empty api key")
	// ErrEmptyAPPKey empty APPKey error
	ErrEmptyAPPKey = errors.New("empty app key")
	// errInitValue used to initialize lastReconcileErr
	errInitValue = errors.New("last error init value")
)

// metricsForwarder sends metrics directly to Datadog using the public API
// its lifecycle must be handled by a ForwardersManager
type metricsForwarder struct {
	id                  string
	datadogClient       *api.Client
	k8sClient           client.Client
	keysHash            uint64
	retryInterval       time.Duration
	sendMetricsInterval time.Duration
	metricsPrefix       string
	globalTags          []string
	tags                []string
	stopChan            chan struct{}
	errorChan           chan error
	eventChan           chan Event
	lastReconcileErr    error
	namespacedName      types.NamespacedName
	logger              logr.Logger
	delegator           delegatedAPI
	decryptor           secrets.Decryptor
	creds               sync.Map
	baseURL             string
	sync.Mutex
	status *datadoghqv1alpha1.DatadogAgentCondition
}

// delegatedAPI is used for testing purpose, it serves for mocking the Datadog API
type delegatedAPI interface {
	delegatedSendDeploymentMetric(float64, string, []string) error
	delegatedSendReconcileMetric(float64, []string) error
	delegatedSendEvent(string, EventType) error
	delegatedValidateCreds(string, string) (*api.Client, error)
}

// newMetricsForwarder returs a new Datadog MetricsForwarder instance
func newMetricsForwarder(k8sClient client.Client, decryptor secrets.Decryptor, obj MonitoredObject) *metricsForwarder {
	return &metricsForwarder{
		id:                  getObjID(obj),
		k8sClient:           k8sClient,
		namespacedName:      getNamespacedName(obj),
		retryInterval:       defaultMetricsRetryInterval,
		sendMetricsInterval: defaultSendMetricsInterval,
		metricsPrefix:       defaultMetricsNamespace,
		stopChan:            make(chan struct{}),
		errorChan:           make(chan error, 100),
		eventChan:           make(chan Event, 10),
		lastReconcileErr:    errInitValue,
		decryptor:           decryptor,
		creds:               sync.Map{},
		baseURL:             defaultbaseURL,
		logger:              log.WithValues("CustomResource.Namespace", obj.GetNamespace(), "CustomResource.Name", obj.GetName()),
	}
}

// start establishes a connection with the Datadog API
// it starts sending deployment metrics once the connection is validated
// designed to run a separate goroutine and stopped using the stop method
func (mf *metricsForwarder) start(wg *sync.WaitGroup) {
	defer wg.Done()

	mf.logger.Info("Starting Datadog metrics forwarder")

	// Global tags need to be set only once
	mf.initGlobalTags()

	// wait.PollImmediateUntil is blocking until mf.connectToDatadogAPI returns true or stopChan is closed
	// wait.PollImmediateUntil keeps retrying to connect to the Datadog API without returning an error
	// wait.PollImmediateUntil returns an error only when stopChan is closed
	if err := wait.PollImmediateUntil(mf.retryInterval, mf.connectToDatadogAPI, mf.stopChan); err == wait.ErrWaitTimeout {
		// stopChan was closed while trying to connect to Datadog API
		// The metrics forwarder stopped by the ForwardersManager
		mf.logger.Info("Shutting down Datadog metrics forwarder")
		return
	}

	mf.logger.Info("Datadog metrics forwarder initialized successfully")

	// Send CR detection event
	crEvent := crDetected(mf.id)
	if err := mf.forwardEvent(crEvent); err != nil {
		mf.logger.Error(err, "an error occurred while sending event")
	}

	metricsTicker := time.NewTicker(mf.sendMetricsInterval)
	defer metricsTicker.Stop()
	for {
		select {
		case <-mf.stopChan:
			// The metrics forwarder is stopped by the ForwardersManager
			// forward metrics and deletion event then return
			if err := mf.forwardMetrics(); err != nil {
				mf.logger.Error(err, "an error occurred while sending metrics")
			}
			crEvent := crDeleted(mf.id)
			if err := mf.forwardEvent(crEvent); err != nil {
				mf.logger.Error(err, "an error occurred while sending event")
			}
			mf.logger.Info("Shutting down Datadog metrics forwarder")
			return
		case <-metricsTicker.C:
			if err := mf.forwardMetrics(); err != nil {
				mf.logger.Error(err, "an error occurred while sending metrics")
			}
		case reconcileErr := <-mf.errorChan:
			if err := mf.processReconcileError(reconcileErr); err != nil {
				mf.logger.Error(err, "an error occurred while processing reconcile metrics")
			}
		case event := <-mf.eventChan:
			if err := mf.forwardEvent(event); err != nil {
				mf.logger.Error(err, "an error occurred while sending event")
			}
		}
	}
}

// stop closes the stopChan to stop the start method
func (mf *metricsForwarder) stop() {
	close(mf.stopChan)
}

func (mf *metricsForwarder) getStatus() *datadoghqv1alpha1.DatadogAgentCondition {
	mf.Lock()
	defer mf.Unlock()
	return mf.status
}

func (mf *metricsForwarder) setStatus(newStatus *datadoghqv1alpha1.DatadogAgentCondition) {
	mf.Lock()
	defer mf.Unlock()
	mf.status = newStatus
}

// connectToDatadogAPI ensures the connection to the Datadog API is valid
// implements wait.ConditionFunc and never returns error to keep retrying
func (mf *metricsForwarder) connectToDatadogAPI() (bool, error) {
	dda, err := mf.getDatadogAgent()
	if err != nil {
		mf.logger.Error(err, "cannot get DatadogAgent to get Datadog credentials,  will retry later...")
		return false, nil
	}
	mf.logger.Info("Getting Datadog credentials")
	apiKey, appKey, err := mf.getCredentials(dda)
	mf.baseURL = getbaseURL(dda)
	mf.logger.Info("Got Datadog Site", "site", mf.baseURL)
	defer mf.updateStatusIfNeeded(err)
	if err != nil {
		mf.logger.Error(err, "cannot get Datadog credentials,  will retry later...")
		return false, nil
	}
	mf.logger.Info("Initializing Datadog metrics forwarder")
	if err = mf.initAPIClient(apiKey, appKey); err != nil {
		mf.logger.Error(err, "cannot get Datadog metrics forwarder to send deployment metrics, will retry later...")
		return false, nil
	}
	return true, nil
}

// forwardMetrics sends metrics to Datadog
// it tries to refresh credentials each time it's called
// forwardMetrics updates status conditions of the Custom Resource
// related to Datadog metrics forwarding by calling updateStatusIfNeeded
func (mf *metricsForwarder) forwardMetrics() error {
	dda, err := mf.getDatadogAgent()
	if err != nil {
		mf.logger.Error(err, "cannot get DatadogAgent to get deployment metrics")
		return err
	}
	apiKey, appKey, err := mf.getCredentials(dda)
	defer mf.updateStatusIfNeeded(err)
	if err != nil {
		mf.logger.Error(err, "cannot get Datadog credentials")
		return err
	}
	if err = mf.updateCredsIfNeeded(apiKey, appKey); err != nil {
		mf.logger.Error(err, "cannot update Datadog credentials")
		return err
	}

	mf.logger.Info("Collecting metrics")
	mf.updateTags(dda)

	// Send status-based metrics
	status := dda.Status.DeepCopy()
	if err = mf.sendStatusMetrics(status); err != nil {
		mf.logger.Error(err, "cannot send status metrics to Datadog")
		return err
	}

	// Send reconcile errors metric
	reconcileErr := mf.getLastReconcileError()
	var metricValue float64
	var tags []string
	metricValue, tags, err = mf.prepareReconcileMetric(reconcileErr)
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

	if reconcileErr == errInitValue {
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

// initAPIClient initializes and validates the Datadog API client
func (mf *metricsForwarder) initAPIClient(apiKey, appKey string) error {
	if mf.delegator == nil {
		mf.delegator = mf
	}
	datadogClient, err := mf.validateCreds(apiKey, appKey)
	if err != nil {
		return err
	}
	mf.datadogClient = datadogClient
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
	datadogClient.SetBaseUrl(mf.baseURL)
	valid, err := datadogClient.Validate()
	if err != nil {
		return nil, fmt.Errorf("cannot validate datadog credentials: %v", err)
	}
	if !valid {
		return nil, fmt.Errorf("invalid datadog credentials on %s", mf.baseURL)
	}
	return datadogClient, nil
}

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

// tagsWithExtraTag used to append an extra tag to the forwarder tags
func (mf *metricsForwarder) tagsWithExtraTag(tagFormat, tag string) []string {
	return append(mf.globalTags, append(mf.tags, fmt.Sprintf(tagFormat, tag))...)
}

// sendDeploymentMetric is a generic method used to forward component deployment metrics to Datadog
func (mf *metricsForwarder) sendDeploymentMetric(metricValue float64, component string, tags []string) error {
	return mf.delegator.delegatedSendDeploymentMetric(metricValue, component, tags)
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

// updateTags updates tags of the DatadogAgent
func (mf *metricsForwarder) updateTags(dda *datadoghqv1alpha1.DatadogAgent) {
	if dda == nil {
		mf.tags = []string{}
		return
	}
	tags := []string{}
	if dda.Spec.ClusterName != "" {
		tags = append(tags, fmt.Sprintf(clusterNameTagFormat, dda.Spec.ClusterName))
	}
	for labelKey, labelValue := range dda.GetLabels() {
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

// getDatadogAgent retrieves the DatadogAgent using Get client method
func (mf *metricsForwarder) getDatadogAgent() (*datadoghqv1alpha1.DatadogAgent, error) {
	dda := &datadoghqv1alpha1.DatadogAgent{}
	err := mf.k8sClient.Get(context.TODO(), mf.namespacedName, dda)
	return dda, err
}

// getCredentials returns the Datadog API Key and APP Key, it returns an error if one key is missing
func (mf *metricsForwarder) getCredentials(dda *datadoghqv1alpha1.DatadogAgent) (string, string, error) {
	var err error
	apiKey, appKey := "", ""

	// Use API key in order of priority: DatadogAgent spec, env var, or secret
	switch {
	case dda.Spec.Credentials.APIKey != "":
		apiKey = dda.Spec.Credentials.APIKey
	case os.Getenv(config.DDAPIKeyEnvVar) != "":
		apiKey = os.Getenv(config.DDAPIKeyEnvVar)
	default:
		secretName, secretKeyName := utils.GetAPIKeySecret(dda)
		apiKey, err = mf.getKeyFromSecret(dda, secretName, secretKeyName)
		if err != nil {
			return "", "", err
		}
	}

	// Use App key in order of priority: DatadogAgent spec, env var, or secret
	switch {
	case dda.Spec.Credentials.AppKey != "":
		appKey = dda.Spec.Credentials.AppKey
	case os.Getenv(config.DDAppKeyEnvVar) != "":
		appKey = os.Getenv(config.DDAppKeyEnvVar)
	default:
		secretName, secretKeyName := utils.GetAppKeySecret(dda)
		appKey, err = mf.getKeyFromSecret(dda, secretName, secretKeyName)
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

	return mf.resolveSecretsIfNeeded(apiKey, appKey)
}

// resolveSecretsIfNeeded calls the secret backend if creds are encrypted
func (mf *metricsForwarder) resolveSecretsIfNeeded(apiKey, appKey string) (string, string, error) {
	if !secrets.IsEnc(apiKey) && !secrets.IsEnc(appKey) {
		// Credentials are not encrypted
		return apiKey, appKey, nil
	}

	// Try to get secrets from the local cache
	if decAPIKey, decAPPKey, cacheHit := mf.getSecretsFromCache(apiKey, appKey); cacheHit {
		// Creds are found in local cache
		return decAPIKey, decAPPKey, nil
	}

	// Cache miss, call the secret decryptor
	decrypted, err := mf.decryptor.Decrypt([]string{apiKey, appKey})
	if err != nil {
		mf.logger.Error(err, "cannot decrypt secrets")
		return "", "", err
	}

	// Update the local cache with the decrypted secrets
	mf.resetSecretsCache(decrypted)

	return decrypted[apiKey], decrypted[appKey], nil
}

// getSecretsFromCache returns the cached and decrypted values of encrypted creds
func (mf *metricsForwarder) getSecretsFromCache(encAPIKey, encAPPKey string) (string, string, bool) {
	decAPIKey, found := mf.creds.Load(encAPIKey)
	if !found {
		return "", "", false
	}

	decAPPKey, found := mf.creds.Load(encAPPKey)
	if !found {
		return "", "", false
	}

	return decAPIKey.(string), decAPPKey.(string), true
}

// resetSecretsCache updates the local secret cache with new secret values
func (mf *metricsForwarder) resetSecretsCache(newSecrets map[string]string) {
	mf.cleanSecretsCache()
	for k, v := range newSecrets {
		mf.creds.Store(k, v)
	}
}

// cleanSecretsCache deletes all cached secrets
func (mf *metricsForwarder) cleanSecretsCache() {
	mf.creds.Range(func(k, v interface{}) bool {
		mf.creds.Delete(k)
		return true
	})
}

// getKeyFromSecret used to retrieve an api or app key from a secret object
func (mf *metricsForwarder) getKeyFromSecret(dda *datadoghqv1alpha1.DatadogAgent, secretName string, dataKey string) (string, error) {
	secret := &corev1.Secret{}
	err := mf.k8sClient.Get(context.TODO(), types.NamespacedName{Namespace: dda.Namespace, Name: secretName}, secret)
	if err != nil {
		return "", err
	}
	return string(secret.Data[dataKey]), nil
}

// updateStatusIfNeeded updates the Datadog metrics forwarding status in the DatadogAgent status
func (mf *metricsForwarder) updateStatusIfNeeded(err error) {
	now := metav1.NewTime(time.Now())
	conditionStatus := corev1.ConditionTrue
	description := "Datadog metrics forwarding ok"
	if err != nil {
		conditionStatus = corev1.ConditionFalse
		description = "Datadog metrics forwarding error"
	}

	oldStatus := mf.getStatus()
	if oldStatus == nil {
		newStatus := condition.NewDatadogAgentStatusCondition(datadoghqv1alpha1.DatadogAgentConditionTypeActiveDatadogMetrics, conditionStatus, now, "", description)
		mf.setStatus(&newStatus)
	} else {
		mf.setStatus(condition.UpdateDatadogAgentStatusCondition(oldStatus, now, datadoghqv1alpha1.DatadogAgentConditionTypeActiveDatadogMetrics, conditionStatus, description))
	}
}

// sendReconcileMetric is used to forward reconcile metrics to Datadog
func (mf *metricsForwarder) sendReconcileMetric(metricValue float64, tags []string) error {
	return mf.delegator.delegatedSendReconcileMetric(metricValue, tags)
}

// delegatedSendReconcileMetric is separated from sendReconcileMetric to facilitate mocking the Datadog API
func (mf *metricsForwarder) delegatedSendReconcileMetric(metricValue float64, tags []string) error {
	ts := float64(time.Now().Unix())
	metricName := fmt.Sprintf(reconcileMetricFormat, mf.metricsPrefix)
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

// forwardEvent sends events to Datadog
func (mf *metricsForwarder) forwardEvent(event Event) error {
	return mf.delegator.delegatedSendEvent(event.Title, event.Type)
}

// delegatedSendEvent is separated from forwardEvent to facilitate mocking the Datadog API
func (mf *metricsForwarder) delegatedSendEvent(eventTitle string, eventType EventType) error {
	event := &api.Event{
		Time:       api.Int(int(time.Now().Unix())),
		Title:      api.String(eventTitle),
		EventType:  api.String(string(eventType)),
		SourceType: api.String(datadogOperatorSourceType),
		Tags:       append(mf.globalTags, mf.tags...),
	}
	if _, err := mf.datadogClient.PostEvent(event); err != nil {
		return err
	}
	return nil
}

// isErrChanFull returs if the errorChan is full
func (mf *metricsForwarder) isErrChanFull() bool {
	return len(mf.errorChan) == cap(mf.errorChan)
}

// isEventChanFull returs if the eventChan is full
func (mf *metricsForwarder) isEventChanFull() bool {
	return len(mf.eventChan) == cap(mf.eventChan)
}

func getbaseURL(dda *datadoghqv1alpha1.DatadogAgent) string {
	if dda.Spec.Agent != nil && dda.Spec.Agent.Config.DDUrl != nil {
		return *dda.Spec.Agent.Config.DDUrl
	} else if dda.Spec.Site != "" {
		return fmt.Sprintf("https://api.%s", dda.Spec.Site)
	}
	return defaultbaseURL
}
