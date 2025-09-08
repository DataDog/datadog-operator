package helm

import (
	"bytes"
	"context"
	"fmt"
	"log"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/storage/driver"
)

func CreateOrUpgrade(ctx context.Context, ac *action.Configuration, releaseName, namespace string, chartData []byte, values map[string]any) error {
	exist, err := doesExist(ctx, ac, releaseName)
	if err != nil {
		return err
	}

	if exist {
		return upgrade(ctx, ac, releaseName, namespace, chartData, values)
	} else {
		return install(ctx, ac, releaseName, namespace, chartData, values)
	}
}

func doesExist(_ context.Context, ac *action.Configuration, releaseName string) (bool, error) {
	historyAction := action.NewHistory(ac)
	historyAction.Max = 1
	versions, err := historyAction.Run(releaseName)

	if err != nil {
		if err == driver.ErrReleaseNotFound || (len(versions) > 0 && versions[len(versions)-1].Info.Status == release.StatusUninstalled) {
			return false, nil
		}
		return false, fmt.Errorf("failed to get Helm release %s history: %w", releaseName, err)
	}

	return true, nil
}

func install(ctx context.Context, ac *action.Configuration, releaseName, namespace string, chartData []byte, values map[string]any) error {
	log.Printf("Installing Helm release %s…", releaseName)

	installAction := action.NewInstall(ac)
	installAction.ReleaseName = releaseName
	installAction.CreateNamespace = true
	installAction.Namespace = namespace

	chart, err := loader.LoadArchive(bytes.NewReader(chartData))
	if err != nil {
		return fmt.Errorf("failed to load Helm chart: %w", err)
	}

	_, err = installAction.RunWithContext(ctx, chart, values)
	if err != nil {
		return fmt.Errorf("failed to install Helm release %s: %w", releaseName, err)
	}

	log.Printf("Installed Helm release %s", releaseName)

	return nil
}

func upgrade(ctx context.Context, ac *action.Configuration, releaseName, namespace string, chartData []byte, values map[string]any) error {
	log.Printf("Upgrading Helm release %s…", releaseName)

	upgradeAction := action.NewUpgrade(ac)
	upgradeAction.Namespace = namespace

	chart, err := loader.LoadArchive(bytes.NewReader(chartData))
	if err != nil {
		return fmt.Errorf("failed to load Helm chart: %w", err)
	}

	_, err = upgradeAction.RunWithContext(ctx, releaseName, chart, values)
	if err != nil {
		return fmt.Errorf("failed to upgrade Helm release %s: %w", releaseName, err)
	}

	log.Printf("Upgraded Helm release %s.", releaseName)

	return nil
}
