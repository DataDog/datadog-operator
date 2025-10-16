// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metadata

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sync"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/pkg/config"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/secrets"
	"github.com/DataDog/datadog-operator/pkg/version"
)

const (
	userAgentHTTPHeaderKey = "User-Agent"

	defaultURLScheme     = "https"
	defaultURLHost       = "app.datadoghq.com"
	defaultURLHostPrefix = "app."
	defaultURLPath       = "api/v1/metadata"

	defaultInterval = 1 * time.Minute
)

type MetadataForwarder struct {
	*SharedMetadata
	credsManager *config.CredentialManager
	client       *http.Client

	requestURL    string
	hostName      string
	creds         sync.Map
	payloadHeader http.Header
	decryptor     secrets.Decryptor

	OperatorMetadata OperatorMetadata
}

type OperatorMetadataPayload struct {
	Hostname  string           `json:"hostname"`
	Timestamp int64            `json:"timestamp"`
	Metadata  OperatorMetadata `json:"datadog_operator_metadata"`
}

type OperatorMetadata struct {
	OperatorVersion               string `json:"operator_version"`
	KubernetesVersion             string `json:"kubernetes_version"`
	InstallMethodTool             string `json:"install_method_tool"`
	InstallMethodToolVersion      string `json:"install_method_tool_version"`
	IsLeader                      bool   `json:"is_leader"`
	DatadogAgentEnabled           bool   `json:"datadogagent_enabled"`
	DatadogMonitorEnabled         bool   `json:"datadogmonitor_enabled"`
	DatadogDashboardEnabled       bool   `json:"datadogdashboard_enabled"`
	DatadogSLOEnabled             bool   `json:"datadogslo_enabled"`
	DatadogGenericResourceEnabled bool   `json:"datadoggenericresource_enabled"`
	DatadogAgentProfileEnabled    bool   `json:"datadogagentprofile_enabled"`
	LeaderElectionEnabled         bool   `json:"leader_election_enabled"`
	ExtendedDaemonSetEnabled      bool   `json:"extendeddaemonset_enabled"`
	RemoteConfigEnabled           bool   `json:"remote_config_enabled"`
	IntrospectionEnabled          bool   `json:"introspection_enabled"`
	ClusterName                   string `json:"cluster_name"`
	ConfigDDURL                   string `json:"config_dd_url"`
	ConfigDDSite                  string `json:"config_site"`
}

var (
	// ErrEmptyAPIKey empty APIKey error
	ErrEmptyAPIKey = errors.New("empty api key")
	// ErrEmptyHostName empty HostName error
	ErrEmptyHostName = errors.New("empty host name")
)

// NewMetadataForwarder creates a new instance of the metadata forwarder
func NewMetadataForwarder(logger logr.Logger, k8sClient client.Reader) *MetadataForwarder {
	return &MetadataForwarder{
		SharedMetadata: NewSharedMetadata(logger, k8sClient),
		hostName:       os.Getenv(constants.DDHostName),
		credsManager:   config.NewCredentialManager(),
		requestURL:     getURL(),
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		OperatorMetadata: OperatorMetadata{},
		decryptor:        secrets.NewSecretBackend(),
	}
}

// Start starts the metadata forwarder
func (mdf *MetadataForwarder) Start() {
	err := mdf.setCredentials()
	if err != nil {
		mdf.logger.Error(err, "Could not set credentials; not starting metadata forwarder")
		return
	}

	if mdf.hostName == "" {
		mdf.logger.Error(ErrEmptyHostName, "Could not set host name; not starting metadata forwarder")
		return
	}

	mdf.payloadHeader = mdf.getHeaders()

	mdf.logger.Info("Starting metadata forwarder")

	ticker := time.NewTicker(defaultInterval)
	go func() {
		for range ticker.C {
			if err := mdf.sendMetadata(); err != nil {
				mdf.logger.Error(err, "Error while sending metadata")
			}
		}
	}()
}

func (mdf *MetadataForwarder) setCredentials() error {
	err := mdf.setupFromOperator()
	if err == nil && mdf.clusterName != "" {
		return nil
	}

	dda, err := mdf.getDatadogAgent()
	if err != nil {
		return err
	}

	return mdf.setupFromDDA(dda)
}

func (mdf *MetadataForwarder) sendMetadata() error {
	payload := mdf.GetPayload()

	mdf.logger.Info("Metadata payload", "payload", string(payload))

	mdf.logger.V(1).Info("Sending metadata to URL", "url", mdf.requestURL)

	reader := bytes.NewReader(payload)
	req, err := http.NewRequestWithContext(context.TODO(), "POST", mdf.requestURL, reader)
	if err != nil {
		mdf.logger.Error(err, "Error creating request", "url", mdf.requestURL, "reader", reader)
		return err
	}
	req.Header = mdf.payloadHeader

	resp, err := mdf.client.Do(req)
	if err != nil {
		return fmt.Errorf("error sending request: %w", err)
	}

	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	mdf.logger.V(1).Info("Read response", "status code", resp.StatusCode, "body", string(body))
	return nil
}

func (mdf *MetadataForwarder) setupFromOperator() error {
	mdf.clusterName = os.Getenv(constants.DDClusterName)

	if mdf.credsManager == nil {
		return fmt.Errorf("credentials Manager is undefined")
	}

	creds, err := mdf.credsManager.GetCredentials()
	if err != nil {
		return err
	}

	// API key
	mdf.apiKey = creds.APIKey

	return nil
}

func (mdf *MetadataForwarder) setupFromDDA(dda *v2alpha1.DatadogAgent) error {
	if mdf.clusterName == "" {
		if dda.Spec.Global != nil && dda.Spec.Global.ClusterName != nil {
			mdf.clusterName = *dda.Spec.Global.ClusterName
		}
	}

	if mdf.apiKey == "" {
		apiKey, err := mdf.getCredentialsFromDDA(dda)
		if err != nil {
			return err
		}
		mdf.apiKey = apiKey

		// if API key is set from DDA, also update request URL if needed
		if dda.Spec.Global != nil {
			if dda.Spec.Global.Site != nil {
				mdfURL := url.URL{
					Scheme: defaultURLScheme,
					Host:   defaultURLHostPrefix + *dda.Spec.Global.Site,
					Path:   defaultURLPath,
				}
				mdf.requestURL = mdfURL.String()
			}
		}
	}

	return nil
}

// getDatadogAgent retrieves the DatadogAgent using Get client method
func (mdf *MetadataForwarder) getDatadogAgent() (*v2alpha1.DatadogAgent, error) {
	// Note: If there are no DDAs present when the Operator starts, the metadata forwarder does not re-try to get credentials from a future DDA
	ddaList := v2alpha1.DatadogAgentList{}

	// Create new client because manager client requires manager to start first
	cfg := ctrl.GetConfigOrDie()
	s := runtime.NewScheme()
	newclient, err := client.New(cfg, client.Options{Scheme: s})
	if err != nil {
		return nil, err
	}
	_ = v2alpha1.AddToScheme(s)

	if err := newclient.List(context.TODO(), &ddaList); err != nil {
		return nil, err
	}

	if len(ddaList.Items) == 0 {
		return nil, errors.New("DatadogAgent not found")
	}

	return &ddaList.Items[0], nil
}

func (mdf *MetadataForwarder) getCredentialsFromDDA(dda *v2alpha1.DatadogAgent) (string, error) {
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
		apiKey, err = mdf.getKeyFromSecret(dda.Namespace, secretName, secretKeyName)
		if err != nil {
			return "", err
		}
	}

	if apiKey == "" {
		return "", ErrEmptyAPIKey
	}

	return mdf.resolveSecretsIfNeeded(apiKey)
}

// getSecretsFromCache returns the cached and decrypted values of encrypted creds
func (mdf *MetadataForwarder) getSecretsFromCache(encAPIKey string) (string, bool) {
	decAPIKey, found := mdf.creds.Load(encAPIKey)
	if !found {
		return "", false
	}

	return decAPIKey.(string), true
}

// resolveSecretsIfNeeded calls the secret backend if creds are encrypted
func (mdf *MetadataForwarder) resolveSecretsIfNeeded(apiKey string) (string, error) {
	if !secrets.IsEnc(apiKey) {
		// Credentials are not encrypted
		return apiKey, nil
	}

	// Try to get secrets from the local cache
	if decAPIKey, cacheHit := mdf.getSecretsFromCache(apiKey); cacheHit {
		// Creds are found in local cache
		return decAPIKey, nil
	}

	// Cache miss, call the secret decryptor
	decrypted, err := mdf.decryptor.Decrypt([]string{apiKey})
	if err != nil {
		mdf.logger.Error(err, "cannot decrypt secrets")
		return "", err
	}

	// Update the local cache with the decrypted secrets
	mdf.resetSecretsCache(decrypted)

	return decrypted[apiKey], nil
}

// resetSecretsCache updates the local secret cache with new secret values
func (mdf *MetadataForwarder) resetSecretsCache(newSecrets map[string]string) {
	mdf.cleanSecretsCache()
	for k, v := range newSecrets {
		mdf.creds.Store(k, v)
	}
}

// cleanSecretsCache deletes all cached secrets
func (mdf *MetadataForwarder) cleanSecretsCache() {
	mdf.creds.Range(func(k, v any) bool {
		mdf.creds.Delete(k)
		return true
	})
}

// getKeyFromSecret is used to retrieve an API or App key from a secret object
func (mdf *MetadataForwarder) getKeyFromSecret(namespace, secretName, dataKey string) (string, error) {
	secret := &corev1.Secret{}
	err := mdf.k8sClient.Get(context.TODO(), types.NamespacedName{Namespace: namespace, Name: secretName}, secret)
	if err != nil {
		return "", err
	}

	return string(secret.Data[dataKey]), nil
}

func (mdf *MetadataForwarder) GetPayload() []byte {
	now := time.Now().Unix()

	mdf.OperatorMetadata.ClusterName = mdf.clusterName
	mdf.OperatorMetadata.OperatorVersion = mdf.operatorVersion
	mdf.OperatorMetadata.KubernetesVersion = mdf.kubernetesVersion

	payload := OperatorMetadataPayload{
		Hostname:  mdf.hostName,
		Timestamp: now,
		Metadata:  mdf.OperatorMetadata,
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		mdf.logger.Error(err, "Error marshaling payload to json")
	}

	return jsonPayload
}

func (mdf *MetadataForwarder) getHeaders() http.Header {
	headers := mdf.GetBaseHeaders()
	headers.Set(userAgentHTTPHeaderKey, fmt.Sprintf("Datadog Operator/%s", version.GetVersion()))
	return headers
}

func getURL() string {
	mdfURL := url.URL{
		Scheme: defaultURLScheme,
		Host:   defaultURLHost,
		Path:   defaultURLPath,
	}

	// check site env var
	// example: datadoghq.com
	if siteFromEnvVar := os.Getenv("DD_SITE"); siteFromEnvVar != "" {
		mdfURL.Host = defaultURLHostPrefix + siteFromEnvVar
	}
	// check url env var
	// example: https://app.datadoghq.com
	if urlFromEnvVar := os.Getenv("DD_URL"); urlFromEnvVar != "" {
		tempURL, err := url.Parse(urlFromEnvVar)
		if err == nil {
			mdfURL.Host = tempURL.Host
			mdfURL.Scheme = tempURL.Scheme
		}
	}

	return mdfURL.String()
}
