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
	"errors"
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
	// hostnamePrefix is the hostname of Operator flares, followed by the
	// cluster ID.
	hostnamePrefix = "datadog-operator"
	// localHTTPTimeout bounds requests to the Operator's own endpoints.
	localHTTPTimeout = 10 * time.Second
)

// CredentialsGetter returns the Operator credentials. Only the API key is
// used. *config.CredentialManager implements it.
type CredentialsGetter interface {
	GetCredentialsForMetadata() (config.Creds, error)
}

// Config configures a Runner.
type Config struct {
	// Reader reads Kubernetes objects. Use the manager's API reader, which
	// doesn't start informers.
	Reader client.Reader
	// Clientset reads pod logs, events and the server version.
	Clientset kubernetes.Interface
	// Credentials returns the API key used to upload flares.
	Credentials CredentialsGetter

	// Namespace and PodName identify the Operator pod. POD_NAMESPACE and
	// POD_NAME (or the hostname) if empty.
	Namespace string
	PodName   string
	// ClusterName is the configured cluster name. DD_CLUSTER_NAME if empty.
	ClusterName string
	// Site and DDURL select the flare intake. DD_SITE, and DD_DD_URL or
	// DD_URL, if empty.
	Site  string
	DDURL string
	// FlareURL overrides the flare intake base URL, for tests.
	FlareURL string

	// MetricsAddr is the bind address of the metrics endpoint, such as
	// :8080. "0" means it's disabled.
	MetricsAddr string
	// SecureMetrics is true if the metrics endpoint is served with HTTPS
	// and authentication. The metrics are then read in-process.
	SecureMetrics bool
	// HealthProbeAddr is the bind address of the health probes, such as
	// :8081.
	HealthProbeAddr string

	// Features lists the controllers and features and whether they are
	// enabled.
	Features map[string]bool
	// LeaderElectionEnabled and LeaderElectionID describe leader election.
	LeaderElectionEnabled bool
	LeaderElectionID      string
	// IsLeader reports whether this replica is the leader.
	IsLeader func() bool
	// RemoteConfigState returns the Remote Configuration client state.
	RemoteConfigState func() any

	// Flags are the command line flags. flag.CommandLine if nil.
	Flags *flag.FlagSet
	// Environ returns the environment. os.Environ if nil.
	Environ func() []string
	// Gatherer reads the metrics in-process. The controller-runtime
	// registry if nil.
	Gatherer prometheus.Gatherer
	// HTTPClient sends the flare and reads the local endpoints.
	HTTPClient *http.Client
	// UploadRetryDelay is the delay between upload attempts.
	UploadRetryDelay time.Duration

	// MaxBytes, Timeout, LogTailLines and LogLimitBytes are the flare
	// limits. The flare package defaults if 0.
	MaxBytes      int64
	Timeout       time.Duration
	LogTailLines  int64
	LogLimitBytes int64

	// Now returns the current time. time.Now if nil.
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
	archiveData, hostname, manifest := r.Collect(ctx, task)
	if archiveData == nil {
		return errors.New("unable to create the flare archive")
	}
	r.logger.Info("Collected the Operator flare", "uuid", task.UUID, "caseID", task.CaseID, "bytes", len(archiveData), "complete", manifest.Complete, "failedCollectors", manifest.FailedCollector, "duration", r.cfg.Now().Sub(start).Round(time.Millisecond).String())

	if err := ctx.Err(); err != nil {
		return fmt.Errorf("the flare task was canceled before the flare was sent: %w", err)
	}
	if r.cfg.Credentials == nil {
		return errors.New("unable to send the flare: no credentials")
	}
	creds, err := r.cfg.Credentials.GetCredentialsForMetadata()
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
		Archive:         archiveData,
	})
	if err != nil {
		return err
	}
	r.logger.Info("Sent the Operator flare", "uuid", task.UUID, "caseID", caseID)
	return nil
}

// Collect collects the flare and returns the zip archive, the flare hostname
// and the manifest.
func (r *Runner) Collect(ctx context.Context, task remoteconfig.FlareTask) ([]byte, string, *flare.Manifest) {
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

	data, err := archive.Close(manifest)
	if err != nil {
		r.logger.Error(err, "Unable to create the flare archive")
		return nil, hostname, manifest
	}
	return data, hostname, manifest
}
