// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flare

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const (
	// DefaultSite is the Datadog site used when none is configured.
	DefaultSite = "datadoghq.com"
	// SourceRemoteConfig is the flare source of flares requested through
	// Remote Configuration (Fleet Automation).
	SourceRemoteConfig = "remote-config"
	supportFlarePath   = "/support/flare"
	uploadAttempts     = 3
	apiKeyHeader       = "Dd-Api-Key"
	httpTimeout        = 60 * time.Second
)

// datadogHostRegexp matches the Datadog hosts whose first label is replaced
// by the versioned flare intake, such as app.datadoghq.com or
// api.us5.datadoghq.com. Other hosts (proxies, fake intakes) are used as is.
var datadogHostRegexp = regexp.MustCompile(`^(app|api)(\.mrf)?\.((us[0-9]+|eu[0-9]*|ap[0-9]+)\.)?(datadoghq\.(com|eu)|ddog-gov\.com|datad0g\.(com|eu))\.?$`)

// FlareURL returns the base URL of the flare intake, such as
// https://1-22-0-flare.agent.datadoghq.com. ddURL, if set, overrides the
// site: a Datadog URL is converted to its flare intake, any other URL is
// used as is.
func FlareURL(site, ddURL, operatorVersion string) (string, error) {
	raw := ddURL
	if raw == "" {
		site = strings.TrimSpace(site)
		if site == "" {
			site = DefaultSite
		}
		raw = "https://app." + site
	}
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", fmt.Errorf("invalid Datadog URL %q: %w", raw, err)
	}
	if u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("invalid Datadog URL %q: missing scheme or host", raw)
	}
	if datadogHostRegexp.MatchString(u.Hostname()) {
		_, domain, _ := strings.Cut(u.Host, ".")
		u.Host = versionDomainPrefix(operatorVersion) + "-flare.agent." + domain
	}
	u.Path = strings.TrimSuffix(u.Path, "/")
	u.RawQuery = ""
	return u.String(), nil
}

var semverRegexp = regexp.MustCompile(`^v?(\d+)\.(\d+)\.(\d+)`)

// versionDomainPrefix returns 1-22-0 for 1.22.0-rc.1, or 0-0-0 when the
// version isn't a semantic version.
func versionDomainPrefix(version string) string {
	m := semverRegexp.FindStringSubmatch(version)
	if m == nil {
		return "0-0-0"
	}
	return m[1] + "-" + m[2] + "-" + m[3]
}

// UploadRequest is a flare to upload. Its fields are sent as the multipart
// fields the Datadog Agent sends (comp/core/flare/helpers/send_flare.go).
type UploadRequest struct {
	// CaseID is the support case the flare is attached to. Empty to create
	// a case.
	CaseID string
	// Email is the email of the user who requested the flare.
	Email string
	// Hostname identifies the flare sender, such as
	// datadog-operator.<cluster_id>.
	Hostname string
	// Source is the flare source, such as remote-config.
	Source string
	// RCTaskUUID is the UUID of the Remote Configuration task that
	// requested the flare.
	RCTaskUUID string
	// FlareSource and Tags are the source and tags of the Remote
	// Configuration task.
	FlareSource string
	Tags        []string
	// OperatorVersion is the version of the Operator.
	OperatorVersion string
	// ArchiveName is the file name of the archive.
	ArchiveName string
	// Archive is the zip archive.
	Archive []byte
}

// Uploader sends flares to the Datadog flare intake.
type Uploader struct {
	// BaseURL is the flare intake, as returned by FlareURL.
	BaseURL string
	// APIKey authenticates the upload.
	APIKey string
	// HTTPClient sends the requests. A client with a 60 second timeout if
	// nil.
	HTTPClient *http.Client
	// RetryDelay is the delay between attempts. 1 second if 0.
	RetryDelay time.Duration
}

type flareResponse struct {
	CaseID      int    `json:"case_id,omitempty"`
	Error       string `json:"error,omitempty"`
	RequestUUID string `json:"request_uuid,omitempty"`
}

// Upload sends a flare and returns the case ID the intake attached it to.
// Like the Datadog Agent, it resolves redirects with a HEAD request, then
// POSTs the flare without following redirects, and retries only when the
// request couldn't be sent (DNS or connection failures).
func (u *Uploader) Upload(ctx context.Context, req UploadRequest) (string, error) {
	if u.APIKey == "" {
		return "", errors.New("unable to send the flare: no API key")
	}
	client := u.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: httpTimeout}
	}
	retryDelay := u.RetryDelay
	if retryDelay <= 0 {
		retryDelay = time.Second
	}

	target := strings.TrimSuffix(u.BaseURL, "/") + supportFlarePath
	if req.CaseID != "" {
		target += "/" + url.PathEscape(req.CaseID)
	}
	target, err := u.resolveURL(ctx, client, target)
	if err != nil {
		return "", err
	}

	noRedirect := *client
	noRedirect.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }

	var lastErr error
	for attempt := 1; attempt <= uploadAttempts; attempt++ {
		resp, err := u.post(ctx, &noRedirect, target, req)
		if err == nil {
			defer resp.Body.Close()
			return analyzeResponse(resp)
		}
		lastErr = err
		if !isRetryableUploadError(err) || attempt == uploadAttempts {
			break
		}
		select {
		case <-ctx.Done():
			return "", fmt.Errorf("unable to send the flare: %w", ctx.Err())
		case <-time.After(retryDelay):
		}
	}
	return "", fmt.Errorf("unable to send the flare: %w", lastErr)
}

func (u *Uploader) resolveURL(ctx context.Context, client *http.Client, target string) (string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodHead, target, nil)
	if err != nil {
		return "", err
	}
	request.Header.Set(apiKeyHeader, u.APIKey)
	resp, err := client.Do(request)
	if err != nil {
		return "", fmt.Errorf("unable to reach the flare intake: %w", err)
	}
	defer resp.Body.Close()
	// The intake expects a POST: a HEAD request ends with 200 or 404.
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
		return "", fmt.Errorf("unable to reach the flare intake %s: %s", stripQuery(target), resp.Status)
	}
	return resp.Request.URL.String(), nil
}

func (u *Uploader) post(ctx context.Context, client *http.Client, target string, req UploadRequest) (*http.Response, error) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	fields := [][2]string{
		{"case_id", req.CaseID},
		{"email", req.Email},
		{"source", req.Source},
		{"rc_task_uuid", req.RCTaskUUID},
		{"flare_source", req.FlareSource},
	}
	for _, field := range fields {
		if field[1] != "" {
			if err := writer.WriteField(field[0], field[1]); err != nil {
				return nil, err
			}
		}
	}
	for _, tag := range req.Tags {
		if err := writer.WriteField("tags", tag); err != nil {
			return nil, err
		}
	}
	name := req.ArchiveName
	if name == "" {
		name = "datadog-operator-flare.zip"
	}
	part, err := writer.CreateFormFile("flare_file", name)
	if err != nil {
		return nil, err
	}
	if _, err = part.Write(req.Archive); err != nil {
		return nil, err
	}
	if req.OperatorVersion != "" {
		if err = writer.WriteField("operator_version", req.OperatorVersion); err != nil {
			return nil, err
		}
	}
	if err = writer.WriteField("hostname", req.Hostname); err != nil {
		return nil, err
	}
	if err = writer.Close(); err != nil {
		return nil, err
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, target, body)
	if err != nil {
		return nil, err
	}
	request.Header.Set(apiKeyHeader, u.APIKey)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	return client.Do(request)
}

func analyzeResponse(resp *http.Response) (string, error) {
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if resp.StatusCode == http.StatusForbidden {
		return "", errors.New("the flare intake refused the upload (HTTP 403 Forbidden): make sure the Operator's API key is valid")
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("the flare intake returned HTTP %s: %s", resp.Status, sample(data))
	}
	if contentType := resp.Header.Get("Content-Type"); !strings.HasPrefix(contentType, "application/json") {
		return "", fmt.Errorf("the flare intake returned HTTP 200 with an unexpected content type %q: %s", contentType, sample(data))
	}
	res := flareResponse{}
	if err := json.Unmarshal(data, &res); err != nil {
		return "", fmt.Errorf("unable to read the flare intake response: %w: %s", err, sample(data))
	}
	if res.Error != "" {
		if res.RequestUUID != "" {
			return "", fmt.Errorf("the flare intake returned an error: %s (request uuid %s)", res.Error, res.RequestUUID)
		}
		return "", fmt.Errorf("the flare intake returned an error: %s", res.Error)
	}
	return fmt.Sprint(res.CaseID), nil
}

func sample(data []byte) string {
	s := strings.TrimSpace(string(data))
	if len(s) > 150 {
		s = s[:150]
	}
	return s
}

func stripQuery(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	u.RawQuery = ""
	return u.String()
}

// isRetryableUploadError reports whether the flare couldn't be sent at all:
// DNS and connection failures. A POST that may have reached the intake isn't
// retried.
func isRetryableUploadError(err error) bool {
	if _, ok := errors.AsType[*net.DNSError](err); ok {
		return true
	}
	if opErr, ok := errors.AsType[*net.OpError](err); ok {
		return opErr.Op == "dial"
	}
	return strings.Contains(err.Error(), "TLS handshake timeout")
}
