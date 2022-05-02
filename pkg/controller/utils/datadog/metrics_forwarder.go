// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadog

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"sync"
	"time"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"
	"github.com/DataDog/datadog-operator/pkg/config"
	"github.com/DataDog/datadog-operator/pkg/controller/utils"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/condition"
	"github.com/DataDog/datadog-operator/pkg/secrets"

	"github.com/go-logr/logr"
	api "github.com/zorkian/go-datadog-api"
	corev1 "k8s.io/api/core/v1"
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
	baseMetricFormat            = "%s.%s"
	clusterNameTagFormat        = "cluster_name:%s"
	crNsTagFormat               = "cr_namespace:%s"
	crNameTagFormat             = "cr_name:%s"
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
	objectKind          string
	logger              logr.Logger
	delegator           delegatedAPI
	decryptor           secrets.Decryptor
	creds               sync.Map
	baseURL             string
	status              *datadoghqv1alpha1.DatadogAgentCondition
	credsManager        *config.CredentialManager
	sync.Mutex
}

// delegatedAPI is used for testing purpose, it serves for mocking the Datadog API
type delegatedAPI interface {
	delegatedSendMonitorMetric(float64, string, []string) error
	delegatedSendDeploymentMetric(float64, string, []string) error
	delegatedSendReconcileMetric(float64, []string) error
	delegatedSendEvent(string, EventType) error
	delegatedValidateCreds(string, string) (*api.Client, error)
}

// newMetricsForwarder returs a new Datadog MetricsForwarder instance
func newMetricsForwarder(k8sClient client.Client, decryptor secrets.Decryptor, obj MonitoredObject, objectKind string) *metricsForwarder {
	return &metricsForwarder{
		id:                  getObjID(obj),
		k8sClient:           k8sClient,
		namespacedName:      getNamespacedName(obj),
		objectKind:          objectKind,
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
		logger:              log.WithValues("CustomResource.Namespace", obj.GetNamespace(), "CustomResource.Name", obj.GetName(), "CustomResource.Kind", objectKind),
		credsManager:        config.NewCredentialManager(),
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
	if err := wait.PollImmediateUntil(mf.retryInterval, mf.connectToDatadogAPI, mf.stopChan); errors.Is(err, wait.ErrWaitTimeout) {
		// stopChan was closed while trying to connect to Datadog API
		// The metrics forwarder stopped by the ForwardersManager
		mf.logger.Info("Shutting down Datadog metrics forwarder, received stop during initialization")
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
				mf.logger.Error(err, "an error occurred while sending final metrics while stopping")
			}
			crEvent := crDeleted(mf.id)
			if err := mf.forwardEvent(crEvent); err != nil {
				mf.logger.Error(err, "an error occurred while sending final events while stopping")
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
	apiKey, appKey, err := mf.getCredentials()
	if err != nil {
		mf.logger.Error(err, "cannot get Datadog credentials, will retry later...")
		return false, nil
	}
	dda, err := mf.getDatadogAgent()
	defer mf.updateStatusIfNeeded(err)
	mf.baseURL = getbaseURL(dda)
	mf.logger.Info("Got Datadog Site", "site", mf.baseURL)

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
	apiKey, appKey, err := mf.getCredentials()
	defer mf.updateStatusIfNeeded(err)
	if err != nil {
		mf.logger.Error(err, "cannot get Datadog credentials")
		return err
	}
	if err = mf.updateCredsIfNeeded(apiKey, appKey); err != nil {
		mf.logger.Error(err, "cannot update Datadog credentials")
		return err
	}

	if err = mf.processMetrics(); err != nil {
		// Specific error is already logged within processMetrics and then passed back up
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
		return nil, fmt.Errorf("cannot validate datadog credentials: %w", err)
	}
	if !valid {
		return nil, fmt.Errorf("invalid datadog credentials on %s", mf.baseURL)
	}

	return datadogClient, nil
}

// tagsWithExtraTag used to append an extra tag to the forwarder tags
func (mf *metricsForwarder) tagsWithExtraTag(tagFormat, tag string) []string {
	return append(mf.globalTags, append(mf.tags, fmt.Sprintf(tagFormat, tag))...)
}

// Sends a gauge typed metric, prefixing the metric name with the set namespace, and aggregate
// the desired tags to use for this metricsForwarder
func (mf *metricsForwarder) gauge(metricName string, metricValue float64, tags []string) error {
	ts := float64(time.Now().Unix())
	fullMetricName := fmt.Sprintf(baseMetricFormat, mf.metricsPrefix, metricName)
	fullTags := append(mf.globalTags, mf.tags...)
	fullTags = append(fullTags, tags...)
	metric := []api.Metric{
		{
			Metric: api.String(fullMetricName),
			Points: []api.DataPoint{
				{
					api.Float64(ts),
					api.Float64(metricValue),
				},
			},
			Type: api.String(gaugeType),
			Tags: fullTags,
		},
	}
	return mf.datadogClient.PostMetrics(metric)
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

// getDatadogMonitor retrieves the DatadogMonitor using Get client method
func (mf *metricsForwarder) getDatadogMonitor() (*datadoghqv1alpha1.DatadogMonitor, error) {
	ddm := &datadoghqv1alpha1.DatadogMonitor{}
	err := mf.k8sClient.Get(context.TODO(), mf.namespacedName, ddm)

	return ddm, err
}

// getCredentials returns the Datadog API Key and APP Key, it returns an error if one key is missing
// getCredentials tries to get the credentials from the DatadogAgent CRD first, then from operator configuration
func (mf *metricsForwarder) getCredentials() (string, string, error) {
	if mf.objectKind == kindDatadogAgent {
		dda, err := mf.getDatadogAgent()
		if err != nil {
			mf.logger.Info("Cannot get DatadogAgent to get Datadog credentials, will retry with Operator Configuration")
			apiKey, appKey, credErr := mf.getCredsFromOperator()
			if credErr != nil {
				mf.logger.Error(credErr, "Cannot get credentials from Operator Configuration")
				return "", "", credErr
			}
			return apiKey, appKey, nil
		}

		apiKey, appKey, err := mf.getCredsFromDatadogAgent(dda)
		if err != nil {
			mf.logger.Error(err, "Cannot get credentials from provided DatadogAgent")
			return "", "", err
		}
		return apiKey, appKey, nil
	}

	apiKey, appKey, err := mf.getCredsFromOperator()
	if err != nil {
		mf.logger.Error(err, "Cannot get credentials from Operator Configuration")
		return "", "", err
	}

	return apiKey, appKey, nil
}

func (mf *metricsForwarder) getCredsFromOperator() (string, string, error) {
	var creds config.Creds
	creds, err := mf.credsManager.GetCredentials()
	return creds.APIKey, creds.AppKey, err
}

func (mf *metricsForwarder) getCredsFromDatadogAgent(dda *datadoghqv1alpha1.DatadogAgent) (string, string, error) {
	var err error
	apiKey, appKey := "", ""

	if dda.Spec.Credentials.APIKey != "" {
		apiKey = dda.Spec.Credentials.APIKey
	} else {
		_, secretName, secretKeyName := utils.GetAPIKeySecret(&dda.Spec.Credentials.DatadogCredentials, utils.GetDefaultCredentialsSecretName(dda))
		apiKey, err = mf.getKeyFromSecret(dda, secretName, secretKeyName)
		if err != nil {
			return "", "", err
		}
	}

	if dda.Spec.Credentials.AppKey != "" {
		appKey = dda.Spec.Credentials.AppKey
	} else {
		_, secretName, secretKeyName := utils.GetAppKeySecret(&dda.Spec.Credentials.DatadogCredentials, utils.GetDefaultCredentialsSecretName(dda))
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
	if mf.objectKind != kindDatadogAgent {
		return
	}

	now := metav1.NewTime(time.Now())
	conditionStatus := corev1.ConditionTrue
	description := "Datadog metrics forwarding ok"
	if err != nil {
		conditionStatus = corev1.ConditionFalse
		description = "Datadog metrics forwarding error"
	}

	if oldStatus := mf.getStatus(); oldStatus == nil {
		newStatus := condition.NewDatadogAgentStatusCondition(datadoghqv1alpha1.DatadogMetricsActive, conditionStatus, now, "", description)
		mf.setStatus(&newStatus)
	} else {
		mf.setStatus(condition.UpdateDatadogAgentStatusCondition(oldStatus, now, datadoghqv1alpha1.DatadogMetricsActive, conditionStatus, description))
	}
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
	if apiutils.BoolValue(dda.Spec.Agent.Enabled) && dda.Spec.Agent.Config != nil && dda.Spec.Agent.Config.DDUrl != nil {
		return *dda.Spec.Agent.Config.DDUrl
	} else if dda.Spec.Site != "" {
		return fmt.Sprintf("https://api.%s", dda.Spec.Site)
	}
	return defaultbaseURL
}
