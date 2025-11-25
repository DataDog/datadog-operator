// Package helm provides Helm chart management functionality for installing
// and upgrading Karpenter releases on Kubernetes clusters.
package helm

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/storage/driver"
)

func CreateOrUpgrade(ctx context.Context, ac *action.Configuration, releaseName, namespace, chartRef, version string, values map[string]any) error {
	exist, err := doesExist(ctx, ac, releaseName)
	if err != nil {
		return err
	}

	if exist {
		return upgrade(ctx, ac, releaseName, namespace, chartRef, version, values)
	} else {
		return install(ctx, ac, releaseName, namespace, chartRef, version, values)
	}
}

func doesExist(_ context.Context, ac *action.Configuration, releaseName string) (bool, error) {
	historyAction := action.NewHistory(ac)
	historyAction.Max = 1
	versions, err := historyAction.Run(releaseName)

	if err != nil {
		if errors.Is(err, driver.ErrReleaseNotFound) || (len(versions) > 0 && versions[len(versions)-1].Info.Status == release.StatusUninstalled) {
			return false, nil
		}
		return false, fmt.Errorf("failed to get Helm release %s history: %w", releaseName, err)
	}

	return true, nil
}

func install(ctx context.Context, ac *action.Configuration, releaseName, namespace, chartRef, version string, values map[string]any) error {
	if version != "" {
		log.Printf("Installing Helm release %s from %s (version: %s)…", releaseName, chartRef, version)
	} else {
		log.Printf("Installing Helm release %s from %s (latest version)…", releaseName, chartRef)
	}

	installAction := action.NewInstall(ac)
	installAction.ReleaseName = releaseName
	installAction.CreateNamespace = true
	installAction.Namespace = namespace
	installAction.ChartPathOptions.Version = version
	installAction.Wait = true
	installAction.WaitForJobs = true
	installAction.Timeout = 30 * time.Minute

	settings := cli.New()
	settings.SetNamespace(namespace)

	chartPath, err := installAction.ChartPathOptions.LocateChart(chartRef, settings)
	if err != nil {
		return fmt.Errorf("failed to locate chart %s: %w", chartRef, err)
	}

	chart, err := loader.Load(chartPath)
	if err != nil {
		return fmt.Errorf("failed to load Helm chart from %s: %w", chartPath, err)
	}

	ctx, cancel := context.WithTimeout(ctx, installAction.Timeout)
	defer cancel()

	release, err := installAction.RunWithContext(ctx, chart, values)
	if err != nil {
		return fmt.Errorf("failed to install Helm release %s: %w", releaseName, err)
	}

	log.Printf("Installed Helm release %s.", release.Name)

	return nil
}

func upgrade(ctx context.Context, ac *action.Configuration, releaseName, namespace, chartRef, version string, values map[string]any) error {
	if version != "" {
		log.Printf("Upgrading Helm release %s from %s (version: %s)…", releaseName, chartRef, version)
	} else {
		log.Printf("Upgrading Helm release %s from %s (latest version)…", releaseName, chartRef)
	}

	upgradeAction := action.NewUpgrade(ac)
	upgradeAction.Namespace = namespace
	upgradeAction.ChartPathOptions.Version = version
	upgradeAction.Wait = true
	upgradeAction.WaitForJobs = true
	upgradeAction.Timeout = 30 * time.Minute

	settings := cli.New()
	settings.SetNamespace(namespace)

	chartPath, err := upgradeAction.ChartPathOptions.LocateChart(chartRef, settings)
	if err != nil {
		return fmt.Errorf("failed to locate chart %s: %w", chartRef, err)
	}

	chart, err := loader.Load(chartPath)
	if err != nil {
		return fmt.Errorf("failed to load Helm chart from %s: %w", chartPath, err)
	}

	ctx, cancel := context.WithTimeout(ctx, upgradeAction.Timeout)
	defer cancel()

	release, err := upgradeAction.RunWithContext(ctx, releaseName, chart, values)
	if err != nil {
		return fmt.Errorf("failed to upgrade Helm release %s: %w", releaseName, err)
	}

	log.Printf("Upgraded Helm release %s.", release.Name)

	return nil
}
