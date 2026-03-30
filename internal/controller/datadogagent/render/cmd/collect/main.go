// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// collect is a helper tool that connects to a Kubernetes cluster, collects
// operator-managed resources for a given DatadogAgent, normalizes them, and
// writes golden YAML files for use with the render comparison test.
//
// Usage:
//
//	go run ./internal/controller/datadogagent/render/cmd/collect/ \
//	  -dda-name datadog \
//	  -namespace datadog \
//	  -output-dir internal/controller/datadogagent/render/testdata/golden/minimal
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	policyv1 "k8s.io/api/policy/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	k8smeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"
	apiregistrationv1 "k8s.io/kube-aggregator/pkg/apis/apiregistration/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

func main() {
	kubeconfig := flag.String("kubeconfig", filepath.Join(os.Getenv("HOME"), ".kube", "config"), "Path to kubeconfig")
	namespace := flag.String("namespace", "datadog", "Namespace where DDA resources live")
	ddaName := flag.String("dda-name", "", "Name of the DatadogAgent resource (required)")
	outputDir := flag.String("output-dir", "", "Directory to write golden files (required)")

	flag.Parse()

	if *ddaName == "" || *outputDir == "" {
		flag.Usage()
		fmt.Fprintln(os.Stderr, "\nError: -dda-name and -output-dir are required")
		os.Exit(1)
	}

	if err := run(*kubeconfig, *namespace, *ddaName, *outputDir); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run(kubeconfig, namespace, ddaName, outputDir string) error {
	ctx := context.Background()

	s := newScheme()
	c, err := buildClient(kubeconfig, s)
	if err != nil {
		return fmt.Errorf("failed to build client: %w", err)
	}

	if err = os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	objects, err1 := collectResources(ctx, c, namespace, ddaName)
	if err1 != nil {
		return fmt.Errorf("failed to collect resources: %w", err1)
	}

	fmt.Fprintf(os.Stderr, "Collected %d resources\n", len(objects))

	for _, obj := range objects {
		normalizeObject(obj)
		setTypeMeta(obj, s)

		gvk := obj.GetObjectKind().GroupVersionKind()
		filename := fmt.Sprintf("%s_%s.yaml", gvk.Kind, obj.GetName())
		path := filepath.Join(outputDir, filename)

		data, err := yaml.Marshal(obj)
		if err != nil {
			return fmt.Errorf("failed to marshal %s/%s: %w", gvk.Kind, obj.GetName(), err)
		}

		if err := os.WriteFile(path, data, 0644); err != nil {
			return fmt.Errorf("failed to write %s: %w", path, err)
		}

		fmt.Fprintf(os.Stderr, "  wrote %s\n", filename)
	}

	return nil
}

func buildClient(kubeconfig string, s *runtime.Scheme) (client.Client, error) {
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, err
	}

	return client.New(config, client.Options{Scheme: s})
}

func collectResources(ctx context.Context, c client.Client, namespace, ddaName string) ([]client.Object, error) {
	var result []client.Object

	// Label selector for namespaced resources
	namespacedSelector := labels.SelectorFromSet(labels.Set{
		kubernetes.AppKubernetesManageByLabelKey: "datadog-operator",
	})

	// Label selector for cluster-scoped resources
	clusterSelector := labels.SelectorFromSet(labels.Set{
		kubernetes.AppKubernetesManageByLabelKey: "datadog-operator",
		kubernetes.AppKubernetesPartOfLabelKey:   ddaName,
	})

	// Namespaced resource types
	namespacedTypes := []client.ObjectList{
		&appsv1.DaemonSetList{},
		&appsv1.DeploymentList{},
		&corev1.ServiceAccountList{},
		&corev1.ServiceList{},
		&corev1.SecretList{},
		&corev1.ConfigMapList{},
		&rbacv1.RoleList{},
		&rbacv1.RoleBindingList{},
		&networkingv1.NetworkPolicyList{},
		&policyv1.PodDisruptionBudgetList{},
	}

	for _, list := range namespacedTypes {
		if err := c.List(ctx, list, client.InNamespace(namespace), client.MatchingLabelsSelector{Selector: namespacedSelector}); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to list %T: %v\n", list, err)
			continue
		}
		items, _ := extractItems(list)
		result = append(result, items...)
	}

	// Cluster-scoped resource types
	clusterTypes := []client.ObjectList{
		&rbacv1.ClusterRoleList{},
		&rbacv1.ClusterRoleBindingList{},
		&apiregistrationv1.APIServiceList{},
		&admissionregistrationv1.MutatingWebhookConfigurationList{},
		&admissionregistrationv1.ValidatingWebhookConfigurationList{},
	}

	for _, list := range clusterTypes {
		if err := c.List(ctx, list, client.MatchingLabelsSelector{Selector: clusterSelector}); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to list %T: %v\n", list, err)
			continue
		}
		items, _ := extractItems(list)
		result = append(result, items...)
	}

	// DatadogAgentInternal objects
	ddaiList := &v1alpha1.DatadogAgentInternalList{}
	if err := c.List(ctx, ddaiList, client.InNamespace(namespace)); err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to list DatadogAgentInternal: %v\n", err)
	} else {
		for i := range ddaiList.Items {
			ddai := &ddaiList.Items[i]
			// Only include DDAIs owned by our DDA
			for _, ref := range ddai.GetOwnerReferences() {
				if ref.Kind == "DatadogAgent" && ref.Name == ddaName {
					result = append(result, ddai)
					break
				}
			}
		}
	}

	return result, nil
}

func extractItems(list client.ObjectList) ([]client.Object, error) {
	items, err := k8smeta.ExtractList(list)
	if err != nil {
		return nil, err
	}

	var result []client.Object
	for _, item := range items {
		if obj, ok := item.(client.Object); ok {
			result = append(result, obj)
		}
	}
	return result, nil
}

// normalizeObject strips fields that differ between a live cluster and render output.
func normalizeObject(obj client.Object) {
	// Same fields as cleanObject in output.go
	obj.SetResourceVersion("")
	obj.SetUID("")
	obj.SetCreationTimestamp(metav1.Time{})
	obj.SetManagedFields(nil)
	obj.SetGeneration(0)
	obj.SetOwnerReferences(nil)

	// Strip annotations that are cluster-specific
	annotations := obj.GetAnnotations()
	if annotations != nil {
		delete(annotations, "kubectl.kubernetes.io/last-applied-configuration")
		if len(annotations) == 0 {
			obj.SetAnnotations(nil)
		} else {
			obj.SetAnnotations(annotations)
		}
	}
}

// setTypeMeta populates apiVersion and kind from the scheme's GVK mappings.
func setTypeMeta(obj client.Object, s *runtime.Scheme) {
	gvks, _, err := s.ObjectKinds(obj)
	if err != nil || len(gvks) == 0 {
		return
	}
	obj.GetObjectKind().SetGroupVersionKind(gvks[0])
}

func newScheme() *runtime.Scheme {
	s := scheme.Scheme
	s.AddKnownTypes(v1alpha1.GroupVersion, &v1alpha1.DatadogAgentInternal{})
	s.AddKnownTypes(v1alpha1.GroupVersion, &v1alpha1.DatadogAgentInternalList{})
	s.AddKnownTypes(v2alpha1.GroupVersion, &v2alpha1.DatadogAgent{})
	s.AddKnownTypes(v2alpha1.GroupVersion, &v2alpha1.DatadogAgentList{})
	s.AddKnownTypes(apiregistrationv1.SchemeGroupVersion, &apiregistrationv1.APIService{})
	s.AddKnownTypes(apiregistrationv1.SchemeGroupVersion, &apiregistrationv1.APIServiceList{})
	return s
}
