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
	"os"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/go-logr/logr"
	api "github.com/zorkian/go-datadog-api"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/pkg/config"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
	"github.com/DataDog/datadog-operator/pkg/secrets"
)

const (
	datadogAgentKind            = "DatadogAgent"
	datadogMonitorKind          = "DatadogMonitor"
	defaultMetricsRetryInterval = 15 * time.Second
	defaultSendMetricsInterval  = 15 * time.Second
	defaultMetricsNamespace     = "datadog.operator"
	gaugeType                   = "gauge"
	countType                   = "count"
	deploymentSuccessValue      = 1.0
	deploymentFailureValue      = 0.0
	deploymentMetricFormat      = "%s.%s.deployment.success"
	stateTagFormat              = "state:%s"
	crAgentNameTagFormat        = "cr_agent_name:%s"
	clusterNameTagFormat        = "kube_cluster_name:%s"
	nsTagFormat                 = "kube_namespace:%s"
	resourceNameTagFormat       = "resource_name:%s"
	crPreferredVersionTagFormat = "cr_preferred_version:%s"
	crOtherVersionTagFormat     = "cr_other_version:%s"
	agentName                   = "agent"
	clusteragentName            = "clusteragent"
	clusterchecksrunnerName     = "clusterchecksrunner"
	reconcileSuccessValue       = 1.0
	reconcileFailureValue       = 0.0
	reconcileMetricFormat       = "%s.reconcile.success"
	reconcileErrTagFormat       = "reconcile_err:%s"
	featureEnabledValue         = 1.0
	featureEnabledFormat        = "%s.%s.feature.enabled"
	customResourceFormat        = "%s.%s.custom_resource.count"
	datadogOperatorSourceType   = "datadog"
	defaultbaseURL              = "https://api.datadoghq.com"
	urlPrefix                   = "https://api."

	// We use an empty application key as solely the API key is necessary to send metrics and events
	emptyAppKey = ""
)

var (
	// ErrEmptyAPIKey empty APIKey error
	ErrEmptyAPIKey = errors.New("empty api key")
	// errInitValue used to initialize lastReconcileErr
	errInitValue = errors.New("last error init value")
)

// datadogForwarderConditionType type use to represent a Datadog Metrics Forwarder condition.
type datadogForwarderConditionType string

const (
	// datadogMetricsActive forwarding metrics and events to Datadog is active.
	datadogMetricsActive datadogForwarderConditionType = "ActiveDatadogMetrics"
	// datadogMetricsError cannot forward deployment metrics and events to Datadog.
	datadogMetricsError datadogForwarderConditionType = "DatadogMetricsError"
)

// delegatedAPI is used for testing purpose, it serves for mocking the Datadog API
type delegatedAPI interface {
	delegatedSendDeploymentMetric(float64, string, []string) error
	delegatedSendReconcileMetric(float64, []string) error
	delegatedSendFeatureMetric(string) error
	delegatedSendEvent(string, EventType) error
	delegatedValidateCreds(string) (*api.Client, error)
}

// hashKeys is used to detect if credentials have changed
// hashKeys is NOT a security function
func hashKeys(apiKey string) uint64 {
	h := fnv.New64()
	_, _ = h.Write([]byte(apiKey))
	return h.Sum64()
}

// metricsForwarder sends metrics directly to Datadog using the public API
// its lifecycle must be handled by a ForwardersManager
type metricsForwarder struct {
	id                  string
	monitoredObjectKind string
	datadogClient       *api.Client
	k8sClient           client.Client

	platformInfo *kubernetes.PlatformInfo
	apiKey       string
	clusterName  string
	labels       map[string]string
	dsStatus     []*v2alpha1.DaemonSetStatus
	dcaStatus    *v2alpha1.DeploymentStatus
	ccrStatus    *v2alpha1.DeploymentStatus

	EnabledFeatures map[string][]string

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
	status              *ConditionCommon
	credsManager        *config.CredentialManager
	sync.RWMutex
}

// newMetricsForwarder returns a new Datadog MetricsForwarder instance
func newMetricsForwarder(k8sClient client.Client, decryptor secrets.Decryptor, obj client.Object, platforminfo *kubernetes.PlatformInfo) *metricsForwarder {
	return &metricsForwarder{
		id:                  getObjID(obj),
		monitoredObjectKind: obj.GetObjectKind().GroupVersionKind().Kind,
		k8sClient:           k8sClient,
		platformInfo:        platforminfo,
		namespacedName:      GetNamespacedName(obj),
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
		credsManager:        config.NewCredentialManager(),
		EnabledFeatures:     make(map[string][]string),
	}
}

// start establishes a connection with the Datadog API
// it starts sending deployment metrics once the connection is validated
// designed to run a separate goroutine and stopped using the stop method
func (mf *metricsForwarder) start(wg *sync.WaitGroup) {
	defer wg.Done()

	mf.logger.Info("Starting Datadog metrics forwarder")

	// wait.PollImmediateUntil is blocking until mf.connectToDatadogAPI returns true or stopChan is closed
	// wait.PollImmediateUntil keeps retrying to connect to the Datadog API without returning an error
	// wait.PollImmediateUntil returns an error only when stopChan is closed
	if err := wait.PollImmediateUntil(mf.retryInterval, mf.connectToDatadogAPI, mf.stopChan); errors.Is(err, wait.ErrWaitTimeout) {
		// stopChan was closed while trying to connect to Datadog API
		// The metrics forwarder stopped by the ForwardersManager
		mf.logger.Info("Shutting down Datadog metrics forwarder")
		return
	}

	// Global tags need to be set only once
	mf.initGlobalTags()

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

// ConditionCommon defines a common struct for the Status Condition
type ConditionCommon struct {
	ConditionType  string
	Status         bool
	LastUpdateTime metav1.Time
	Reason         string
	Message        string
}

// stop closes the stopChan to stop the start method
func (mf *metricsForwarder) stop() {
	close(mf.stopChan)
}

// getStatus returns the status of the metrics forwarder
func (mf *metricsForwarder) getStatus() *ConditionCommon {
	mf.RLock()
	defer mf.RUnlock()
	return mf.status
}

func (mf *metricsForwarder) setStatus(newStatus *ConditionCommon) {
	mf.Lock()
	defer mf.Unlock()
	mf.status = newStatus
}

func (mf *metricsForwarder) setup() error {
	credsSet := mf.setupFromOperator()

	// If this is a DDA forwarder, then need to set status metrics from DDA even if credentials were set by the Operator
	if mf.monitoredObjectKind == datadogAgentKind {
		dda, err := mf.getDatadogAgent()
		if err != nil {
			mf.logger.Error(err, "cannot retrieve DatadogAgent to get Datadog credentials, will retry later...")
			return err
		}
		return mf.setupFromDDA(dda, credsSet)
	}

	return nil

}

func (mf *metricsForwarder) setupFromOperator() bool {
	if mf.credsManager == nil {
		return false
	}

	creds, err := mf.credsManager.GetCredentials()
	if err != nil {
		return false
	}

	// API key
	mf.apiKey = creds.APIKey

	// base URL
	mf.baseURL = defaultbaseURL
	mf.logger.V(1).Info("Got API URL for the Datadog Operator", "site", mf.baseURL)
	if os.Getenv(constants.DDddURL) != "" {
		mf.baseURL = os.Getenv(constants.DDddURL)
	} else if os.Getenv(constants.DDURL) != "" {
		mf.baseURL = os.Getenv(constants.DDURL)
	} else if site := os.Getenv(constants.DDSite); site != "" {
		mf.baseURL = urlPrefix + strings.TrimSpace(site)
	}

	// cluster name
	mf.clusterName = os.Getenv(constants.DDClusterName)
	return true
}

func (mf *metricsForwarder) setupFromDDA(dda *v2alpha1.DatadogAgent, credsSetFromOperator bool) error {
	if !credsSetFromOperator {
		mf.baseURL = getbaseURL(dda)
		mf.logger.V(1).Info("Got API URL for DatadogAgent", "site", mf.baseURL)

		// set apiKey
		apiKey, err := mf.getCredentialsFromDDA(dda)
		if err != nil {
			return err
		}
		mf.apiKey = apiKey
	}

	mf.labels = dda.GetLabels()

	status := dda.Status.DeepCopy()
	mf.dsStatus = status.AgentList
	mf.dcaStatus = status.ClusterAgent
	mf.ccrStatus = status.ClusterChecksRunner

	if mf.clusterName == "" && dda.Spec.Global != nil && dda.Spec.Global.ClusterName != nil {
		mf.clusterName = *dda.Spec.Global.ClusterName
	}

	return nil
}

// connectToDatadogAPI ensures the connection to the Datadog API is valid
// implements wait.ConditionFunc and never returns error to keep retrying
func (mf *metricsForwarder) connectToDatadogAPI() (bool, error) {
	var err error
	err = mf.setup()

	defer mf.updateStatusIfNeeded(err)
	if err != nil {
		mf.logger.Error(err, "cannot get Datadog credentials,  will retry later...")
		return false, nil
	}
	mf.logger.Info("Initializing Datadog metrics forwarder")
	if err = mf.initAPIClient(mf.apiKey); err != nil {
		mf.logger.Error(err, "cannot retrieve Datadog metrics forwarder to send deployment metrics, will retry later...")
		return false, nil
	}
	return true, nil
}

// forwardMetrics sends metrics to Datadog
// it tries to refresh credentials each time it's called
// forwardMetrics updates status conditions of the Custom Resource
// related to Datadog metrics forwarding by calling updateStatusIfNeeded
func (mf *metricsForwarder) forwardMetrics() error {
	var err error
	err = mf.setup()

	defer mf.updateStatusIfNeeded(err)
	if err != nil {
		mf.logger.Error(err, "cannot get Datadog credentials")
		return err
	}
	if err = mf.updateCredsIfNeeded(mf.apiKey); err != nil {
		mf.logger.Error(err, "cannot update Datadog credentials")
		return err
	}

	mf.logger.V(1).Info("Collecting metrics")

	// Send status-based metrics
	if err = mf.sendStatusMetrics(mf.dsStatus, mf.dcaStatus, mf.ccrStatus); err != nil {
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

	// send feature metrics
	for _, featuresList := range mf.EnabledFeatures {
		for _, feature := range featuresList {
			mf.sendFeatureMetric(feature)
		}
	}

	mf.sendResourceCountMetric()

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
	tags = append(tags, mf.getCRVersionTags()...)
	return metricValue, tags, nil
}

// getLastReconcileError provides thread-safe read access to lastReconcileErr
func (mf *metricsForwarder) getLastReconcileError() error {
	mf.RLock()
	defer mf.RUnlock()
	return mf.lastReconcileErr
}

// setLastReconcileError provides thread-safe write access to lastReconcileErr
func (mf *metricsForwarder) setLastReconcileError(newErr error) {
	mf.Lock()
	defer mf.Unlock()
	mf.lastReconcileErr = newErr
}

// initAPIClient initializes and validates the Datadog API client
func (mf *metricsForwarder) initAPIClient(apiKey string) error {
	if mf.delegator == nil {
		mf.delegator = mf
	}
	datadogClient, err := mf.validateCreds(apiKey)
	if err != nil {
		return err
	}
	mf.datadogClient = datadogClient
	mf.keysHash = hashKeys(apiKey)
	return nil
}

// updateCredsIfNeeded used to update Datadog apiKey if it changes
func (mf *metricsForwarder) updateCredsIfNeeded(apiKey string) error {
	if mf.keysHash != hashKeys(apiKey) {
		return mf.initAPIClient(apiKey)
	}
	return nil
}

// validateCreds returns validates the API key by querying the Datadog API
func (mf *metricsForwarder) validateCreds(apiKey string) (*api.Client, error) {
	return mf.delegator.delegatedValidateCreds(apiKey)
}

// delegatedValidateCreds is separated from validateCreds to facilitate mocking the Datadog API
func (mf *metricsForwarder) delegatedValidateCreds(apiKey string) (*api.Client, error) {
	datadogClient := api.NewClient(apiKey, emptyAppKey)
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

func (mf *metricsForwarder) sendStatusMetrics(dsStatus []*v2alpha1.DaemonSetStatus, dcaStatus, ccrStatus *v2alpha1.DeploymentStatus) error {
	var metricValue float64

	// Agent deployment metrics
	if len(dsStatus) > 0 {
		for _, status := range dsStatus {
			if status.Available == status.Desired {
				metricValue = deploymentSuccessValue
			} else {
				metricValue = deploymentFailureValue
			}
			tags := mf.tagsWithExtraTag(stateTagFormat, status.State)
			tags = append(tags, mf.getCRVersionTags()...)
			tags = append(tags, fmt.Sprintf(crAgentNameTagFormat, status.DaemonsetName))
			if err := mf.sendDeploymentMetric(metricValue, agentName, tags); err != nil {
				return err
			}
		}
	}

	// Cluster Agent deployment metrics
	if dcaStatus != nil {
		if dcaStatus.AvailableReplicas == dcaStatus.Replicas {
			metricValue = deploymentSuccessValue
		} else {
			metricValue = deploymentFailureValue
		}
		tags := mf.tagsWithExtraTag(stateTagFormat, dcaStatus.State)
		tags = append(tags, mf.getCRVersionTags()...)
		if err := mf.sendDeploymentMetric(metricValue, clusteragentName, tags); err != nil {
			return err
		}
	}

	// Cluster Check Runner deployment metrics
	if ccrStatus != nil {
		if ccrStatus.AvailableReplicas == ccrStatus.Replicas {
			metricValue = deploymentSuccessValue
		} else {
			metricValue = deploymentFailureValue
		}
		tags := mf.tagsWithExtraTag(stateTagFormat, ccrStatus.State)
		tags = append(tags, mf.getCRVersionTags()...)
		if err := mf.sendDeploymentMetric(metricValue, clusterchecksrunnerName, tags); err != nil {
			return err
		}
	}

	return nil
}

// tagsWithExtraTag used to append an extra tag to the forwarder tags
func (mf *metricsForwarder) tagsWithExtraTag(tagFormat, tag string) []string {
	return append(mf.globalTags, append(mf.tags, fmt.Sprintf(tagFormat, tag))...)
}

// getDatadogAgentCRVersionTags returns DatadogAgent CRD version tags
func (mf *metricsForwarder) getCRVersionTags() []string {
	ddaPreferredVersion, ddaOtherVersion := mf.platformInfo.GetApiVersions(mf.monitoredObjectKind)

	versionTags := []string{}

	if ddaPreferredVersion == "" {
		// This should never happen, since forwarder is created for an object created by Kubernetes, implying support for that API.
		ddaPreferredVersion = "null"
	}
	versionTags = append(versionTags, fmt.Sprintf(crPreferredVersionTagFormat, ddaPreferredVersion))
	if ddaOtherVersion != "" {
		versionTags = append(versionTags, fmt.Sprintf(crOtherVersionTagFormat, ddaOtherVersion))
	}
	return versionTags
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

// initGlobalTags defines the Custom Resource namespace and name tags
func (mf *metricsForwarder) initGlobalTags() {
	mf.globalTags = append(mf.globalTags, []string{
		fmt.Sprintf(nsTagFormat, mf.namespacedName.Namespace),
		fmt.Sprintf(resourceNameTagFormat, mf.namespacedName.Name),
	}...)

	if mf.clusterName != "" {
		mf.globalTags = append(mf.globalTags, fmt.Sprintf(clusterNameTagFormat, mf.clusterName))
	}
}

// getDatadogAgent retrieves the DatadogAgent using Get client method
func (mf *metricsForwarder) getDatadogAgent() (*v2alpha1.DatadogAgent, error) {
	dda := &v2alpha1.DatadogAgent{}
	err := mf.k8sClient.Get(context.TODO(), mf.namespacedName, dda)

	return dda, err
}

// getCredentialsFromDDA retrieves the API key configured in the DatadogAgent
func (mf *metricsForwarder) getCredentialsFromDDA(dda *v2alpha1.DatadogAgent) (string, error) {
	if dda.Spec.Global == nil || dda.Spec.Global.Credentials == nil {
		return "", fmt.Errorf("credentials not configured in the DatadogAgent")
	}

	defaultSecretName := secrets.GetDefaultCredentialsSecretName(dda)

	var err error
	apiKey := ""

	if dda.Spec.Global != nil && dda.Spec.Global.Credentials != nil && dda.Spec.Global.Credentials.APIKey != nil && *dda.Spec.Global.Credentials.APIKey != "" {
		apiKey = *dda.Spec.Global.Credentials.APIKey
	} else {
		_, secretName, secretKeyName := secrets.GetAPIKeySecret(dda.Spec.Global.Credentials, defaultSecretName)
		apiKey, err = mf.getKeyFromSecret(dda.Namespace, secretName, secretKeyName)
		if err != nil {
			return "", err
		}
	}

	if apiKey == "" {
		return "", ErrEmptyAPIKey
	}

	return mf.resolveSecretsIfNeeded(apiKey)
}

// resolveSecretsIfNeeded calls the secret backend if creds are encrypted
func (mf *metricsForwarder) resolveSecretsIfNeeded(apiKey string) (string, error) {
	if !secrets.IsEnc(apiKey) {
		// Credentials are not encrypted
		return apiKey, nil
	}

	// Try to get secrets from the local cache
	if decAPIKey, cacheHit := mf.getSecretsFromCache(apiKey); cacheHit {
		// Creds are found in local cache
		return decAPIKey, nil
	}

	// Cache miss, call the secret decryptor
	decrypted, err := mf.decryptor.Decrypt([]string{apiKey})
	if err != nil {
		mf.logger.Error(err, "cannot decrypt secrets")
		return "", err
	}

	// Update the local cache with the decrypted secrets
	mf.resetSecretsCache(decrypted)

	return decrypted[apiKey], nil
}

// getSecretsFromCache returns the cached and decrypted values of encrypted creds
func (mf *metricsForwarder) getSecretsFromCache(encAPIKey string) (string, bool) {
	decAPIKey, found := mf.creds.Load(encAPIKey)
	if !found {
		return "", false
	}

	return decAPIKey.(string), true
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

// getKeyFromSecret is used to retrieve an API or App key from a secret object
func (mf *metricsForwarder) getKeyFromSecret(namespace, secretName, dataKey string) (string, error) {
	secret := &corev1.Secret{}
	err := mf.k8sClient.Get(context.TODO(), types.NamespacedName{Namespace: namespace, Name: secretName}, secret)
	if err != nil {
		return "", err
	}

	return string(secret.Data[dataKey]), nil
}

// updateStatusIfNeeded updates the Datadog metrics forwarding status in the DatadogAgent status
func (mf *metricsForwarder) updateStatusIfNeeded(err error) {
	now := metav1.NewTime(time.Now())
	conditionStatus := true
	message := "Datadog metrics forwarding ok"
	reason := "MetricsForwardingSucceeded"
	conditionType := string(datadogMetricsActive)

	if err != nil {
		conditionStatus = false
		message = "Datadog metrics forwarding error"
		reason = "MetricsForwardingError"
		conditionType = string(datadogMetricsError)
	}

	newConditionStatus := &ConditionCommon{
		ConditionType:  conditionType,
		Status:         conditionStatus,
		LastUpdateTime: now,
		Message:        message,
		Reason:         reason,
	}
	mf.setStatus(newConditionStatus)
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

// sendFeatureMetric is used to forward feature enabled metrics to Datadog
func (mf *metricsForwarder) sendFeatureMetric(feature string) error {
	return mf.delegator.delegatedSendFeatureMetric(feature)
}

// delegatedSendFeatureMetric is separated from sendFeatureMetric to facilitate mocking the Datadog API
func (mf *metricsForwarder) delegatedSendFeatureMetric(feature string) error {
	ts := float64(time.Now().Unix())
	metricName := fmt.Sprintf(featureEnabledFormat, mf.metricsPrefix, feature)
	series := []api.Metric{
		{
			Metric: api.String(metricName),
			Points: []api.DataPoint{
				{
					api.Float64(ts),
					api.Float64(featureEnabledValue),
				},
			},
			Type: api.String(gaugeType),
			Tags: mf.globalTags,
		},
	}
	return mf.datadogClient.PostMetrics(series)
}

var objectKindToSnake = map[string]string{
	datadogAgentKind:   "datadog_agent",
	datadogMonitorKind: "datadog_monitor",
}

func (mf *metricsForwarder) sendResourceCountMetric() error {
	// At start mf.monitoredObjectKind may be empty; don't send metric in this case
	if _, ok := objectKindToSnake[mf.monitoredObjectKind]; !ok {
		return nil
	}

	ts := float64(time.Now().Unix())
	metricName := fmt.Sprintf(customResourceFormat, mf.metricsPrefix, objectKindToSnake[mf.monitoredObjectKind])
	tags := append(mf.tags, mf.globalTags...)
	series := []api.Metric{
		{
			Metric: api.String(metricName),
			Points: []api.DataPoint{
				{
					api.Float64(ts),
					// Each forwarder corresponds to one resource, so always submit a count of 1
					api.Float64(1),
				},
			},
			Type: api.String(countType),
			Tags: tags,
		},
	}

	return mf.datadogClient.PostMetrics(series)
}

// isErrChanFull returs if the errorChan is full
func (mf *metricsForwarder) isErrChanFull() bool {
	return len(mf.errorChan) == cap(mf.errorChan)
}

// isEventChanFull returs if the eventChan is full
func (mf *metricsForwarder) isEventChanFull() bool {
	return len(mf.eventChan) == cap(mf.eventChan)
}

func getbaseURL(dda *v2alpha1.DatadogAgent) string {
	if dda.Spec.Global != nil && dda.Spec.Global.Endpoint != nil && dda.Spec.Global.Endpoint.URL != nil {
		return *dda.Spec.Global.Endpoint.URL
	} else if dda.Spec.Global != nil && dda.Spec.Global.Site != nil && *dda.Spec.Global.Site != "" {
		return urlPrefix + *dda.Spec.Global.Site
	}
	return defaultbaseURL
}

// setEnabledFeatures updates the list of enabled features for a namespaced object
func (mf *metricsForwarder) setEnabledFeatures(features []string) {
	mf.Lock()
	defer mf.Unlock()

	if len(mf.EnabledFeatures) == 0 {
		mf.EnabledFeatures = make(map[string][]string)
	}

	mf.EnabledFeatures[mf.id] = features
}
