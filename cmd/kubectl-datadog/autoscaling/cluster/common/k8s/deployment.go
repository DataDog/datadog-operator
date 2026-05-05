package k8s

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/go-containerregistry/pkg/name"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/pager"
)

// FindFirstDeployment scans Deployments cluster-wide and returns the first one
// for which predicate returns true, or nil if none matches. It uses the
// client-go pager defaults and short-circuits on the first match via a
// sentinel error so the caller never materialises the whole Deployment set.
//
// The cluster-wide enumeration order is unspecified, so when several matches
// coexist which one wins is non-deterministic. Callers that expect at most one
// match (e.g. cluster-autoscaler, Karpenter controller) can rely on this; for
// "all matches" use cases, list with a label selector instead.
func FindFirstDeployment(
	ctx context.Context,
	client kubernetes.Interface,
	predicate func(appsv1.Deployment) bool,
) (*appsv1.Deployment, error) {
	errFound := errors.New("first match")

	var found *appsv1.Deployment
	p := pager.New(func(ctx context.Context, opts metav1.ListOptions) (runtime.Object, error) {
		return client.AppsV1().Deployments(metav1.NamespaceAll).List(ctx, opts)
	})
	err := p.EachListItem(ctx, metav1.ListOptions{}, func(obj runtime.Object) error {
		dep := obj.(*appsv1.Deployment)
		if predicate(*dep) {
			found = dep
			return errFound
		}
		return nil
	})
	if err != nil && !errors.Is(err, errFound) {
		return nil, fmt.Errorf("failed to list Deployments: %w", err)
	}
	return found, nil
}

// ExtractDeploymentVersion returns the running version of the controller
// matching containerMatch. Prefers the image tag of the matching container
// (the source of truth) and falls back to the `app.kubernetes.io/version`
// label on the Deployment or its pod template (set by most Helm charts).
// Empty when neither is available — e.g. an image referenced by digest only
// and no version label.
func ExtractDeploymentVersion(d appsv1.Deployment, containerMatch func(corev1.Container) bool) string {
	for _, c := range d.Spec.Template.Spec.Containers {
		if !containerMatch(c) {
			continue
		}
		if tag := imageTag(c.Image); tag != "" {
			return tag
		}
	}
	if v := d.Labels["app.kubernetes.io/version"]; v != "" {
		return v
	}
	return d.Spec.Template.Labels["app.kubernetes.io/version"]
}

// imageTag extracts the tag portion of an OCI image reference. Returns empty
// when no tag is set (digest-only references or bare image names).
func imageTag(image string) string {
	// pkg/name parses a combined `tag@digest` reference as a Digest, dropping
	// the tag — strip the digest first so the tag is preserved.
	if i := strings.Index(image, "@"); i >= 0 {
		image = image[:i]
	}
	// WithDefaultTag("") suppresses the implicit `latest` so a missing tag
	// surfaces as an empty TagStr rather than the default.
	ref, err := name.ParseReference(image, name.WeakValidation, name.WithDefaultTag(""))
	if err != nil {
		return ""
	}
	if t, ok := ref.(name.Tag); ok {
		return t.TagStr()
	}
	return ""
}
