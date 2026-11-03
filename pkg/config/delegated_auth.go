// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	datadogapi "github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/aws/aws-sdk-go-v2/aws"
	awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"

	"github.com/DataDog/datadog-operator/pkg/constants"
)

// delegatedTokenRefreshMargin is how long before expiration a cached delegated
// token is considered stale and gets renewed.
const delegatedTokenRefreshMargin = 2 * time.Minute

const (
	// delegatedTokenRequestTimeout bounds a whole token exchange: AWS credential retrieval
	// plus the Datadog token request. Exchanges are serialized behind the token cache lock,
	// so an unbounded exchange would block every reconcile that needs a token.
	delegatedTokenRequestTimeout = 30 * time.Second
	// awsCredentialsHTTPTimeout bounds each HTTP request the AWS SDK makes to obtain
	// credentials (EKS Pod Identity agent, STS for IRSA, instance metadata). The SDK
	// credentials cache refreshes without the caller's cancellation, so this is what
	// ends a hung refresh.
	awsCredentialsHTTPTimeout = 10 * time.Second
	// maxDelegatedTokenResponseSize caps the token endpoint response read into memory.
	maxDelegatedTokenResponseSize = 1 << 20
)

// DelegatedAuthSettings configures cloud-provider based ("delegated") authentication
// against the Datadog API. When enabled, the operator exchanges a signed proof of its
// cloud identity (e.g. an AWS IAM role obtained through EKS Pod Identity or IRSA) for a
// short-lived Datadog access token, removing the need for a Datadog application key.
type DelegatedAuthSettings struct {
	// Provider is the cloud provider type. Only "aws" is supported.
	Provider string
	// OrgUUID is the UUID of the Datadog organization the cloud identity is linked to.
	OrgUUID string
	// Region is the AWS region whose STS endpoint is used for the identity proof.
	// When empty, the region resolved by the AWS SDK is used; if none, the global endpoint.
	Region string
}

// DelegatedAuthSettingsFromEnv reads delegated auth settings from the environment.
// Like the Datadog Agent, delegated auth is enabled when an org UUID is set.
// It returns (nil, nil) when delegated auth is not configured.
func DelegatedAuthSettingsFromEnv() (*DelegatedAuthSettings, error) {
	orgUUID := strings.TrimSpace(os.Getenv(constants.DDDelegatedAuthOrgUUID))
	if orgUUID == "" {
		return nil, nil
	}

	// AWS is the only supported provider, so an empty value defaults to it.
	provider := strings.ToLower(strings.TrimSpace(os.Getenv(constants.DDDelegatedAuthProvider)))
	if provider == "" {
		provider = datadogapi.ProviderAWS
	}
	if provider != datadogapi.ProviderAWS {
		return nil, fmt.Errorf("unsupported %s %q, supported values: %q", constants.DDDelegatedAuthProvider, provider, datadogapi.ProviderAWS)
	}

	return &DelegatedAuthSettings{
		Provider: provider,
		OrgUUID:  orgUUID,
		Region:   strings.TrimSpace(os.Getenv(constants.DDDelegatedAuthAWSRegion)),
	}, nil
}

// awsCredentialsLoader returns AWS credentials and the region resolved by the SDK.
type awsCredentialsLoader func(ctx context.Context) (aws.Credentials, string, error)

// defaultAWSCredentialsLoader resolves credentials through the AWS SDK default chain, which
// supports EKS Pod Identity (container credentials endpoint), IRSA (web identity token),
// static environment variables, shared config and instance metadata.
func defaultAWSCredentialsLoader() awsCredentialsLoader {
	var (
		once    sync.Once
		cfg     aws.Config
		loadErr error
	)
	return func(ctx context.Context) (aws.Credentials, string, error) {
		once.Do(func() {
			// LoadDefaultConfig wraps the provider chain in an aws.CredentialsCache,
			// so credentials are only re-fetched when they are about to expire.
			cfg, loadErr = awsconfig.LoadDefaultConfig(context.Background(),
				awsconfig.WithHTTPClient(awshttp.NewBuildableClient().WithTimeout(awsCredentialsHTTPTimeout)),
			)
		})
		if loadErr != nil {
			return aws.Credentials{}, "", fmt.Errorf("unable to load AWS configuration: %w", loadErr)
		}
		if cfg.Credentials == nil {
			return aws.Credentials{}, "", errors.New("no AWS credentials provider configured")
		}
		creds, err := cfg.Credentials.Retrieve(ctx)
		if err != nil {
			return aws.Credentials{}, "", fmt.Errorf("unable to retrieve AWS credentials: %w", err)
		}
		return creds, cfg.Region, nil
	}
}

// awsDelegatedTokenProvider implements datadogapi.DelegatedTokenProvider using AWS credentials
// resolved by the AWS SDK. The built-in datadogapi.AWSAuth only reads static credentials from
// environment variables, which are not available with EKS Pod Identity or IRSA.
type awsDelegatedTokenProvider struct {
	region          string
	loadCredentials awsCredentialsLoader
	httpClient      *http.Client
	// timeout bounds the whole exchange; zero means delegatedTokenRequestTimeout.
	timeout time.Duration
}

var _ datadogapi.DelegatedTokenProvider = &awsDelegatedTokenProvider{}

// Authenticate signs an STS GetCallerIdentity request with the pod's AWS credentials and
// exchanges it for a Datadog delegated token. The context must carry the Datadog server
// variables so the token is requested from the configured Datadog site.
func (p *awsDelegatedTokenProvider) Authenticate(ctx context.Context, config *datadogapi.DelegatedTokenConfig) (*datadogapi.DelegatedTokenCredentials, error) {
	if config == nil || config.OrgUUID == "" {
		return nil, errors.New("missing org UUID in delegated token config")
	}

	timeout := p.timeout
	if timeout <= 0 {
		timeout = delegatedTokenRequestTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	awsCreds, sdkRegion, err := p.loadCredentials(ctx)
	if err != nil {
		return nil, err
	}

	region := p.region
	if region == "" {
		region = sdkRegion
	}

	awsAuth := &datadogapi.AWSAuth{AwsRegion: region}
	proof, err := awsAuth.GenerateAwsAuthData(config.OrgUUID, &datadogapi.Credentials{
		AccessKeyID:     awsCreds.AccessKeyID,
		SecretAccessKey: awsCreds.SecretAccessKey,
		SessionToken:    awsCreds.SessionToken,
	})
	if err != nil {
		return nil, fmt.Errorf("unable to generate AWS identity proof: %w", err)
	}

	authString := strings.Join([]string{proof.BodyEncoded, proof.HeadersEncoded, proof.Method, proof.URLEncoded}, "|")
	token, err := p.requestDelegatedToken(ctx, config.OrgUUID, authString)
	if err != nil {
		return nil, fmt.Errorf("unable to exchange AWS identity for a Datadog delegated token: %w", err)
	}
	return token, nil
}

// requestDelegatedToken exchanges the identity proof for a delegated token. It mirrors
// datadogapi.GetDelegatedToken, which has no timeout, drops the server error detail, and
// echoes the response (including the token) in its parse errors.
func (p *awsDelegatedTokenProvider) requestDelegatedToken(ctx context.Context, orgUUID, proof string) (*datadogapi.DelegatedTokenCredentials, error) {
	url, err := datadogapi.GetDelegatedTokenUrl(ctx)
	if err != nil {
		return nil, fmt.Errorf("unable to resolve the delegated token URL: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(nil))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Delegated "+proof)

	httpClient := p.httpClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: delegatedTokenRequestTimeout}
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxDelegatedTokenResponseSize))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token request failed: %s%s", resp.Status, apiErrorDetail(body))
	}
	return parseDelegatedTokenResponse(body, orgUUID, proof)
}

// parseDelegatedTokenResponse reads the token from a successful response. Unlike
// datadogapi.ParseDelegatedTokenResponse, its errors never include response content,
// which may hold the token and would otherwise end up in logs.
func parseDelegatedTokenResponse(body []byte, orgUUID, proof string) (*datadogapi.DelegatedTokenCredentials, error) {
	var payload struct {
		Data struct {
			Attributes struct {
				AccessToken string          `json:"access_token"`
				Expires     json.RawMessage `json:"expires"`
			} `json:"attributes"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		// json errors can quote offending values; report only the failure class.
		return nil, errors.New("malformed delegated token response: invalid JSON")
	}
	attrs := payload.Data.Attributes
	if attrs.AccessToken == "" {
		return nil, errors.New("malformed delegated token response: missing data.attributes.access_token")
	}

	// Same default as the API client when the expiration is missing or unparsable.
	expiration := time.Now().Add(15 * time.Minute)
	if unix, ok := parseUnixSeconds(attrs.Expires); ok {
		expiration = time.Unix(unix, 0)
	}

	return &datadogapi.DelegatedTokenCredentials{
		OrgUUID:        orgUUID,
		DelegatedToken: attrs.AccessToken,
		DelegatedProof: proof,
		Expiration:     expiration,
	}, nil
}

// parseUnixSeconds accepts a Unix timestamp encoded as a JSON string or number.
func parseUnixSeconds(raw json.RawMessage) (int64, bool) {
	if len(raw) == 0 {
		return 0, false
	}
	var str string
	if err := json.Unmarshal(raw, &str); err == nil {
		v, err := strconv.ParseInt(str, 10, 64)
		return v, err == nil
	}
	var num int64
	if err := json.Unmarshal(raw, &num); err == nil {
		return num, true
	}
	return 0, false
}

// apiErrorDetail extracts the server-authored JSON:API error title/detail (for example,
// "no identity mapping found" for an unmapped IAM role) without echoing the raw body.
func apiErrorDetail(body []byte) string {
	var payload struct {
		Errors []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return ""
	}
	var details []string
	for _, e := range payload.Errors {
		msg := strings.TrimSpace(strings.Join([]string{e.Title, e.Detail}, ": "))
		msg = strings.Trim(msg, ": ")
		if msg != "" {
			details = append(details, msg)
		}
	}
	if len(details) == 0 {
		return ""
	}
	return " (" + strings.Join(details, "; ") + ")"
}

// delegatedTokenSource caches a delegated token and renews it shortly before it expires.
// It is safe for concurrent use.
type delegatedTokenSource struct {
	config *datadogapi.DelegatedTokenConfig
	now    func() time.Time

	mu    sync.Mutex
	token *datadogapi.DelegatedTokenCredentials
}

func newDelegatedTokenSource(settings *DelegatedAuthSettings) *delegatedTokenSource {
	return &delegatedTokenSource{
		config: &datadogapi.DelegatedTokenConfig{
			OrgUUID:  settings.OrgUUID,
			Provider: settings.Provider,
			ProviderAuth: &awsDelegatedTokenProvider{
				region:          settings.Region,
				loadCredentials: defaultAWSCredentialsLoader(),
			},
		},
		now: time.Now,
	}
}

// Token returns a valid delegated token, fetching a new one if the cached token is
// missing or close to expiration. The returned value is a copy and may be stored in
// a request context without being shared across goroutines.
func (s *delegatedTokenSource) Token(ctx context.Context) (*datadogapi.DelegatedTokenCredentials, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.token == nil || s.token.DelegatedToken == "" || s.now().Add(delegatedTokenRefreshMargin).After(s.token.Expiration) {
		token, err := s.config.ProviderAuth.Authenticate(ctx, s.config)
		if err != nil {
			return nil, err
		}
		if token == nil || token.DelegatedToken == "" {
			return nil, errors.New("empty delegated token returned by Datadog")
		}
		s.token = token
	}

	tokenCopy := *s.token
	return &tokenCopy, nil
}
