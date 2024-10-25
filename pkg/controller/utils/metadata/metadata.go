package metadata

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"

	"github.com/go-logr/logr"
)

const (
	apiHTTPHeaderKey       = "Dd-Api-Key"
	useragentHTTPHeaderKey = "User-Agent"
	contentTypeHeaderKey   = "Content-Type"
	acceptHeaderKey        = "Accept"

	defaultURLScheme     = "https"
	defaultURLHost       = "app.datadog.com"
	defaultURLHostPrefix = "app."
	defaultURLPath       = "api/v1/metadata"

	defaultInterval = 1 * time.Minute
)

type MetadataForwarder struct {
	OperatorMetadata OperatorMetadata
	logger           logr.Logger
	stopChan         chan struct{}
}

// NewMetadataForwarder creates a new instance of the metadata forwarder
func NewMetadataForwarder(logger logr.Logger) *MetadataForwarder {
	return &MetadataForwarder{
		OperatorMetadata: OperatorMetadata{},
		logger:           logger,
		stopChan:         make(chan struct{}),
	}
}

// Start starts the metadata forwarder
func (mdf *MetadataForwarder) Start() {
	ticker := time.NewTicker(getTickerInterval(mdf.logger))
	go func() {
		for {
			select {
			case <-mdf.stopChan:
				ticker.Stop()
				mdf.logger.Info("Stopping ticker for metadata forwarder case")
				return
			case <-ticker.C:
				if err := mdf.sendMetadata(); err != nil {
					mdf.logger.Error(err, "Error while sending metadata")
				}
			}
		}
	}()
}

// Stop stops the metadata forwarder
func (mdf *MetadataForwarder) Stop() {
	close(mdf.stopChan)
	mdf.logger.Info("Stopping metadata forwarder")
}

func (mdf *MetadataForwarder) sendMetadata() error {
	url := createURL(mdf.logger)
	mdf.logger.V(1).Info("Sending metadata to URL", "url", url)

	reader := bytes.NewReader(mdf.createPayload())
	req, err := http.NewRequestWithContext(context.TODO(), "POST", url, reader)
	if err != nil {
		mdf.logger.Error(err, "Error creating request", "url", url, "reader", reader)
	}
	req.Header = getHeaders()

	client := &http.Client{
		Timeout: 10 * time.Second,
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("Error sending request: %w", err)
	}

	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("Failed to read response body: %w", err)
	}
	mdf.logger.V(1).Info("Read response", "status code", resp.StatusCode, "body", string(body))
	return nil
}

func getHeaders() http.Header {
	header := http.Header{}
	header.Set(apiHTTPHeaderKey, os.Getenv("DD_API_KEY"))
	header.Set(useragentHTTPHeaderKey, fmt.Sprintf("datadog-operator/%s", "test")) // TODO: use operator version
	header.Set(contentTypeHeaderKey, "application/json")
	header.Set(acceptHeaderKey, "application/json")
	return header
}

func createURL(logger logr.Logger) string {
	url := url.URL{
		Scheme: defaultURLScheme,
		Host:   defaultURLHost,
		Path:   defaultURLPath,
	}
	// prioritize URL set in env var
	if mdURLFromEnvVar := os.Getenv("METADATA_URL"); mdURLFromEnvVar != "" {
		parsedURL, err := url.Parse(mdURLFromEnvVar)
		if err != nil {
			logger.Error(err, "Error parsing METADATA_URL")
		}
		// TODO: check for correct URL format
		return parsedURL.String()
	}
	// check site env var
	// example: datadoghq.com
	if siteFromEnvVar := os.Getenv("DD_SITE"); siteFromEnvVar != "" {
		url.Host = defaultURLHostPrefix + siteFromEnvVar
	}
	// check url env var
	// example: https://app.datadoghq.com
	if urlFromEnvVar := os.Getenv("DD_URL"); urlFromEnvVar != "" {
		url.Host = urlFromEnvVar
	}

	return url.String()
}

func getTickerInterval(logger logr.Logger) time.Duration {
	interval := defaultInterval
	if s := os.Getenv("METADATA_INTERVAL"); s != "" {
		i, err := strconv.Atoi(s)
		if err != nil {
			logger.Error(err, "Error coverting METADATA_INTERVAL")
		}
		interval = time.Duration(i) * time.Minute
	}
	logger.Info("Operator metadata will be sent periodically", "frequency (seconds)", interval)
	return interval
}
