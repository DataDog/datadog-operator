// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package dashboard

import (
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Standard Helm metadata.
const (
	helmChartLabel            = "helm.sh/chart"
	helmManagedByLabel        = "app.kubernetes.io/managed-by"
	helmReleaseNameAnnotation = "meta.helm.sh/release-name"
	helmReleaseNSAnnotation   = "meta.helm.sh/release-namespace"
)

// ReleaseRef is a Helm release.
type ReleaseRef struct {
	Namespace string
	Name      string
}

// Key returns the key of the release in Snapshot.Helm.
func (r ReleaseRef) Key() string { return ReleaseKey(r.Namespace, r.Name) }

// HelmRelease returns the Helm release managing obj, read from the
// meta.helm.sh annotations. The release namespace defaults to the object's.
func HelmRelease(obj metav1.Object) (ReleaseRef, bool) {
	ann := obj.GetAnnotations()
	name := ann[helmReleaseNameAnnotation]
	if name == "" {
		return ReleaseRef{}, false
	}
	ns := ann[helmReleaseNSAnnotation]
	if ns == "" {
		ns = obj.GetNamespace()
	}
	return ReleaseRef{Namespace: ns, Name: name}, true
}

// isHelmManaged reports whether obj carries standard Helm metadata.
func isHelmManaged(obj metav1.Object) bool {
	labels := obj.GetLabels()
	_, hasRelease := HelmRelease(obj)
	return hasRelease || labels[helmChartLabel] != "" || labels[helmManagedByLabel] == "Helm"
}

// parseChartLabel splits a helm.sh/chart label "<name>-<version>". The
// version starts at the first "-<digit>" followed by a dotted remainder, so
// that chart names with dashes are kept whole.
func parseChartLabel(label string) (chart, version string) {
	for i := 0; i+1 < len(label); i++ {
		if label[i] == '-' && label[i+1] >= '0' && label[i+1] <= '9' && strings.Contains(label[i+1:], ".") {
			// Helm replaces "+" with "_" in the label value.
			return label[:i], strings.ReplaceAll(label[i+1:], "_", "+")
		}
	}
	return label, ""
}

// helmDeploySkew is how long before the rollout start the release may have
// been deployed and still be the cause of the rollout.
const helmDeploySkew = 2 * time.Minute

// buildHelm returns the Helm info of a Helm-managed object, or nil. The
// current chart comes from the newest release revision when the history is
// readable, from the helm.sh/chart label otherwise. The previous revision is
// added when the newest one was deployed for the rollout started at
// rolloutStart; a zero rolloutStart means no rollout.
func buildHelm(s *Snapshot, obj metav1.Object, rolloutStart time.Time) *HelmView {
	if !isHelmManaged(obj) {
		return nil
	}
	hv := &HelmView{}
	hv.Chart, hv.Version = parseChartLabel(obj.GetLabels()[helmChartLabel])
	rel, ok := HelmRelease(obj)
	if !ok {
		return hv
	}
	hv.Release, hv.Namespace = rel.Name, rel.Namespace
	revs := s.Helm[rel.Key()]
	if len(revs) == 0 {
		return hv
	}
	hv.Chart, hv.Version, hv.Revision = revs[0].Chart, revs[0].ChartVersion, revs[0].Revision
	if !rolloutStart.IsZero() && len(revs) > 1 && !revs[0].LastDeployed.Before(rolloutStart.Add(-helmDeploySkew)) {
		hv.PreviousVersion, hv.PreviousRevision = revs[1].ChartVersion, revs[1].Revision
	}
	return hv
}
