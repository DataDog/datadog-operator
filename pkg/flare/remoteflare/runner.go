// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package remoteflare collects and sends Operator flares requested through
// Remote Configuration (Fleet Automation). The Operator collects its own
// control-plane view in-process, with its Kubernetes client.
package remoteflare

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"

	"github.com/DataDog/datadog-operator/pkg/config"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/flare"
	"github.com/DataDog/datadog-operator/pkg/remoteconfig"
	"github.com/DataDog/datadog-operator/pkg/version"
)

const (
	// hostnamePrefix is followed by the cluster ID.
	hostnamePrefix   = "datadog-operator"
	localHTTPTimeout = 10 * time.Second
)

// CredentialsGetter is implemented by *config.CredentialManager.
type CredentialsGetter interface {
	GetAPIKeyCredentials() (config.Creds, error)
}

// Config configures a Runner. Empty fields get the defaults NewRunner sets.
type Config struct {
	// Reader should be the manager's API reader, which starts no informers.
	Reader      client.Reader
	Clientset   kubernetes.Interface
	Credentials CredentialsGetter

	Namespace   string
	PodName     string
	ClusterName string
	// Site and DDURL select the flare intake; FlareURL overrides it in tests.
	Site     string
	DDURL    string
	FlareURL string

	// MetricsAddr "0" disables the metrics endpoint. With SecureMetrics, the
	// metrics are read in-process.
	MetricsAddr     string
	SecureMetrics   bool
	HealthProbeAddr string

	Features              map[string]bool
	LeaderElectionEnabled bool
	LeaderElectionID      string
	IsLeader              func() bool
	RemoteConfigState     func() any
	RemoteConfigOrg       func() remoteconfig.FlareOrg

	Flags            *flag.FlagSet
	Environ          func() []string
	Gatherer         prometheus.Gatherer
	HTTPClient       *http.Client
	UploadRetryDelay time.Duration

	// MaxBytes, Timeout, LogTailLines and LogLimitBytes default to the flare
	// package defaults.
	MaxBytes      int64
	Timeout       time.Duration
	LogTailLines  int64
	LogLimitBytes int64

	Now func() time.Time
}

// Runner collects and sends Operator flares. It implements
// remoteconfig.FlareRunner.
type Runner struct {
	cfg    Config
	logger logr.Logger
}

var _ remoteconfig.FlareRunner = (*Runner)(nil)

// NewRunner returns a Runner.
func NewRunner(cfg Config, logger logr.Logger) *Runner {
	if cfg.Namespace == "" {
		cfg.Namespace = os.Getenv("POD_NAMESPACE")
	}
	if cfg.PodName == "" {
		cfg.PodName = os.Getenv("POD_NAME")
	}
	if cfg.PodName == "" {
		cfg.PodName, _ = os.Hostname()
	}
	if cfg.ClusterName == "" {
		cfg.ClusterName = os.Getenv(constants.DDClusterName)
	}
	if cfg.Site == "" {
		cfg.Site = os.Getenv(constants.DDSite)
	}
	if cfg.DDURL == "" {
		cfg.DDURL = os.Getenv(constants.DDddURL)
	}
	if cfg.DDURL == "" {
		cfg.DDURL = os.Getenv(constants.DDURL)
	}
	if cfg.Flags == nil {
		cfg.Flags = flag.CommandLine
	}
	if cfg.Environ == nil {
		cfg.Environ = os.Environ
	}
	if cfg.Gatherer == nil {
		cfg.Gatherer = ctrlmetrics.Registry
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Timeout: 60 * time.Second}
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &Runner{cfg: cfg, logger: logger}
}

// Run collects a flare and sends it to the flare intake for task. A flare
// is sent even if some collectors fail or the time limit is reached: the
// manifest in the archive lists the failed collectors. Run returns an error
// only if the flare couldn't be sent.
func (r *Runner) Run(ctx context.Context, task remoteconfig.FlareTask) error {
	start := r.cfg.Now()
	archivePath, hostname, manifest, err := r.Collect(ctx, task)
	if err != nil {
		return err
	}
	defer os.Remove(archivePath)
	var size int64
	if info, statErr := os.Stat(archivePath); statErr == nil {
		size = info.Size()
	}
	r.logger.Info("Collected the Operator flare", "uuid", task.UUID, "caseID", task.CaseID, "bytes", size, "complete", manifest.Complete, "failedCollectors", manifest.FailedCollector, "duration", r.cfg.Now().Sub(start).Round(time.Millisecond).String())

	if ctxErr := ctx.Err(); ctxErr != nil {
		return fmt.Errorf("the flare task was canceled before the flare was sent: %w", ctxErr)
	}
	creds, err := r.cfg.Credentials.GetAPIKeyCredentials()
	if err != nil {
		return fmt.Errorf("unable to send the flare: unable to get the API key: %w", err)
	}
	baseURL := r.cfg.FlareURL
	if baseURL == "" {
		if baseURL, err = flare.FlareURL(r.cfg.Site, r.cfg.DDURL, version.Version); err != nil {
			return fmt.Errorf("unable to send the flare: %w", err)
		}
	}

	uploader := &flare.Uploader{BaseURL: baseURL, APIKey: creds.APIKey, HTTPClient: r.cfg.HTTPClient, RetryDelay: r.cfg.UploadRetryDelay}
	caseID, err := uploader.Upload(ctx, flare.UploadRequest{
		CaseID:          task.CaseID,
		Email:           task.UserHandle,
		Hostname:        hostname,
		Source:          flare.SourceRemoteConfig,
		RCTaskUUID:      task.UUID,
		FlareSource:     task.Source,
		Tags:            task.Tags,
		OperatorVersion: version.Version,
		ArchiveName:     fmt.Sprintf("datadog-operator-flare-%s.zip", start.UTC().Format("2006-01-02-15-04-05")),
		ArchivePath:     archivePath,
	})
	if err != nil {
		return err
	}
	r.logger.Info("Sent the Operator flare", "uuid", task.UUID, "caseID", caseID)
	return nil
}

// Collect collects the flare and returns the path of the zip archive, which
// the caller removes, the flare hostname and the manifest.
func (r *Runner) Collect(ctx context.Context, task remoteconfig.FlareTask) (string, string, *flare.Manifest, error) {
	timeout := r.cfg.Timeout
	if timeout <= 0 {
		timeout = flare.DefaultTimeout
	}

	env := flare.Env{
		Reader:            r.cfg.Reader,
		Clientset:         r.cfg.Clientset,
		OperatorNamespace: r.cfg.Namespace,
		ClusterName:       r.cfg.ClusterName,
		LogTailLines:      r.cfg.LogTailLines,
		LogLimitBytes:     r.cfg.LogLimitBytes,
	}

	// The pod and cluster ID are needed by several collectors and the
	// upload. They're best effort: collectors report what's missing.
	setupCtx, cancelSetup := context.WithTimeout(ctx, 10*time.Second)
	pod, podErr := r.operatorPod(setupCtx)
	env.OperatorPod = pod
	clusterID, clusterIDErr := flare.ClusterID(setupCtx, r.cfg.Reader)
	cancelSetup()
	hostname := hostnamePrefix + "." + clusterID
	if clusterID == "" {
		hostname = hostnamePrefix + ".unknown-cluster"
	}

	archive := flare.NewArchive(flare.ArchiveOptions{Root: hostname, MaxBytes: r.cfg.MaxBytes, Now: r.cfg.Now})
	closed := false
	defer func() {
		// Remove the archive file if collection panicked.
		if !closed {
			archive.Discard()
		}
	}()
	collectors := r.operatorCollectors()
	collectors = append(collectors, flare.ConfigCollectors()...)
	collectors = append(collectors, flare.DatadogResourcesCollectors()...)
	collectors = append(collectors, flare.WorkloadsCollector(), flare.ClusterCollector())

	manifest := flare.Run(ctx, env, archive, collectors, flare.RunOptions{Timeout: timeout, Now: r.cfg.Now})
	manifest.OperatorVersion = version.Version
	manifest.Info = map[string]string{
		"hostname":     hostname,
		"source":       flare.SourceRemoteConfig,
		"rc_task_uuid": task.UUID,
		"case_id":      task.CaseID,
		"namespace":    r.cfg.Namespace,
		"pod":          r.cfg.PodName,
	}
	if podErr != nil {
		manifest.Info["pod_error"] = podErr.Error()
	}
	if clusterIDErr != nil {
		manifest.Info["cluster_id_error"] = clusterIDErr.Error()
	}
	manifest.Org = r.orgInfo(task)

	archivePath, err := archive.Close(manifest)
	closed = true
	if err != nil {
		return "", hostname, manifest, fmt.Errorf("unable to create the flare archive: %w", err)
	}
	return archivePath, hostname, manifest, nil
}

// orgInfo returns the org the flare came from: the site the Operator is
// configured with, the org UUID the Remote Configuration client fetched from
// the org data endpoint and stored in its database, and the org ID of the
// task's Remote Configuration path.
func (r *Runner) orgInfo(task remoteconfig.FlareTask) *flare.OrgInfo {
	info := &flare.OrgInfo{Site: flare.SiteFromSettings(r.cfg.Site, r.cfg.DDURL, "the Operator environment")}

	if orgID, err := remoteconfig.ConfigPathOrgID(task.ConfigPath); err != nil {
		info.OrgID = flare.UnknownOrgValue(err.Error())
	} else {
		info.OrgID = flare.Known(orgID, "the Remote Configuration path of the flare task, "+task.ConfigPath)
	}

	org := r.cfg.RemoteConfigOrg()
	if org.Site != "" && org.Site != info.Site.Value {
		info.Site.Source += fmt.Sprintf("; the Remote Configuration client uses the site %s", org.Site)
	}
	if org.OrgUUID != "" {
		info.OrgUUID = flare.Known(org.OrgUUID, "the Remote Configuration database: the org UUID the client fetched from the org data endpoint")
	} else {
		info.OrgUUID = flare.UnknownOrgValue(org.OrgUUIDError)
	}
	info.RemoteConfigOrgEnabled = org.OrgEnabled
	info.RemoteConfigAPIKeyAuthorized = org.APIKeyAuthorized
	return info
}
