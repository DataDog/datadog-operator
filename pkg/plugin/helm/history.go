// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package helm provides read-only access to Helm release metadata for the
// kubectl-datadog plugin.
package helm

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/storage/driver"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

// ErrReleaseNotFound is returned when the release has no history in the namespace.
var ErrReleaseNotFound = errors.New("helm release not found")

// Revision is the metadata of one release revision. It carries no values,
// config or manifest, so release secrets never reach the caller.
type Revision struct {
	Chart        string
	ChartVersion string
	Revision     int
	Status       string
	LastDeployed time.Time
}

// Storage drivers History can read, as named by the HELM_DRIVER variable of
// the Helm CLI.
const (
	DriverSecret    = "secret"
	DriverConfigMap = "configmap"
)

// History returns up to limit revisions of the release (all when limit <= 0),
// newest first, from the Secret storage driver.
func History(ctx context.Context, getter genericclioptions.RESTClientGetter, namespace, release string, limit int) ([]Revision, error) {
	return HistoryFrom(ctx, getter, DriverSecret, namespace, release, limit)
}

// HistoryFrom is History reading the given HELM_DRIVER storage driver; only
// the secret and configmap drivers are supported.
func HistoryFrom(ctx context.Context, getter genericclioptions.RESTClientGetter, driver, namespace, release string, limit int) ([]Revision, error) {
	driver, err := storageDriver(driver)
	if err != nil {
		return nil, err
	}
	cfg := new(action.Configuration)
	if err := cfg.Init(getter, namespace, driver, noopLog); err != nil {
		return nil, fmt.Errorf("failed to initialize Helm configuration: %w", err)
	}
	return history(ctx, cfg, release, limit)
}

// storageDriver normalizes a HELM_DRIVER value, accepting only the drivers
// backed by Kubernetes objects.
func storageDriver(driver string) (string, error) {
	switch driver {
	case "", "secret", "secrets":
		return DriverSecret, nil
	case "configmap", "configmaps":
		return DriverConfigMap, nil
	default:
		return "", fmt.Errorf("unsupported Helm storage driver %q", driver)
	}
}

// history runs the History action on an existing configuration so that tests
// can inject one backed by Helm's in-memory storage driver.
func history(ctx context.Context, cfg *action.Configuration, release string, limit int) ([]Revision, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	releases, err := action.NewHistory(cfg).Run(release)
	if err != nil {
		if errors.Is(err, driver.ErrReleaseNotFound) {
			return nil, fmt.Errorf("%w: %s", ErrReleaseNotFound, release)
		}
		return nil, fmt.Errorf("failed to get Helm release %s history: %w", release, err)
	}

	revisions := make([]Revision, 0, len(releases))
	for _, rel := range releases {
		if rel == nil {
			continue
		}
		rev := Revision{Revision: rel.Version}
		if rel.Chart != nil && rel.Chart.Metadata != nil {
			rev.Chart = rel.Chart.Metadata.Name
			rev.ChartVersion = rel.Chart.Metadata.Version
		}
		if rel.Info != nil {
			rev.Status = rel.Info.Status.String()
			rev.LastDeployed = rel.Info.LastDeployed.Time
		}
		revisions = append(revisions, rev)
	}

	slices.SortFunc(revisions, func(a, b Revision) int { return cmp.Compare(b.Revision, a.Revision) })
	if limit > 0 && len(revisions) > limit {
		revisions = revisions[:limit]
	}
	return revisions, nil
}

func noopLog(string, ...any) {}
