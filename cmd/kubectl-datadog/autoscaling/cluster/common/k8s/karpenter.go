package k8s

import (
	"context"
	"fmt"

	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

var karpenterAPIGroups = map[string]bool{
	"karpenter.sh":      true,
	"karpenter.k8s.aws": true,
}

const helmReleaseNameKarpenter = "karpenter"

// DetectActiveKarpenter checks for an active Karpenter installation by looking for
// webhook configurations that reference Karpenter API groups. Returns the namespace
// where Karpenter's webhook service is running.
func DetectActiveKarpenter(ctx context.Context, clientset kubernetes.Interface) (bool, string, error) {
	vwcList, err := clientset.AdmissionregistrationV1().ValidatingWebhookConfigurations().List(ctx, metav1.ListOptions{})
	if err != nil {
		return false, "", fmt.Errorf("failed to list ValidatingWebhookConfigurations: %w", err)
	}

	for _, vwc := range vwcList.Items {
		for _, webhook := range vwc.Webhooks {
			if ns, ok := extractKarpenterNamespace(webhook.Rules, webhook.ClientConfig); ok {
				return true, ns, nil
			}
		}
	}

	mwcList, err := clientset.AdmissionregistrationV1().MutatingWebhookConfigurations().List(ctx, metav1.ListOptions{})
	if err != nil {
		return false, "", fmt.Errorf("failed to list MutatingWebhookConfigurations: %w", err)
	}

	for _, mwc := range mwcList.Items {
		for _, webhook := range mwc.Webhooks {
			if ns, ok := extractKarpenterNamespace(webhook.Rules, webhook.ClientConfig); ok {
				return true, ns, nil
			}
		}
	}

	return false, "", nil
}

func extractKarpenterNamespace(rules []admissionregistrationv1.RuleWithOperations, clientConfig admissionregistrationv1.WebhookClientConfig) (string, bool) {
	for _, rule := range rules {
		for _, group := range rule.APIGroups {
			if karpenterAPIGroups[group] {
				if clientConfig.Service != nil {
					return clientConfig.Service.Namespace, true
				}
				// URL-based webhook — Karpenter is present but we can't
				// determine the namespace (e.g. external/out-of-cluster).
				return "", true
			}
		}
	}
	return "", false
}

// FindKarpenterHelmRelease searches for a deployed Helm release named "karpenter"
// across all namespaces by looking at Helm storage secrets. This is a fallback
// for when webhooks are absent (e.g. pods crashed) but the Helm release still exists.
func FindKarpenterHelmRelease(ctx context.Context, clientset kubernetes.Interface) (bool, []string, error) {
	secrets, err := clientset.CoreV1().Secrets("").List(ctx, metav1.ListOptions{
		LabelSelector: "owner=helm,name=" + helmReleaseNameKarpenter + ",status=deployed",
	})
	if err != nil {
		return false, nil, fmt.Errorf("failed to list Helm release secrets: %w", err)
	}

	if len(secrets.Items) == 0 {
		return false, nil, nil
	}

	// Deduplicate namespaces (multiple revisions may exist in the same namespace)
	seen := map[string]bool{}
	var namespaces []string
	for _, s := range secrets.Items {
		if !seen[s.Namespace] {
			seen[s.Namespace] = true
			namespaces = append(namespaces, s.Namespace)
		}
	}

	return true, namespaces, nil
}
