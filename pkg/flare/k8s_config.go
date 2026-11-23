// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flare

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ConfigCollectors returns the collectors of the configuration the Operator
// manages or runs with. Secrets are never collected.
func ConfigCollectors() []Collector {
	return []Collector{
		{Name: "config/configmaps", Collect: collectConfigMaps},
		{Name: "config/rbac", Collect: collectRBAC},
		{Name: "config/webhooks", Collect: collectWebhooks},
		{Name: "config/crds", Collect: collectCRDs},
		{Name: "operator/deployment", Collect: collectOperatorDeployment},
	}
}

func collectConfigMaps(ctx context.Context, c *Collection) error {
	configMaps := &corev1.ConfigMapList{}
	if err := c.Env.Reader.List(ctx, configMaps, client.MatchingLabels{managedByLabel: managedByOperator}, client.Limit(maxListItems)); err != nil {
		return fmt.Errorf("unable to list ConfigMaps: %w", err)
	}
	for i := range configMaps.Items {
		cm := &configMaps.Items[i]
		cleanTyped(cm)
		// Binary data isn't readable and can't be scrubbed line by line.
		for key, value := range cm.BinaryData {
			cm.BinaryData[key] = []byte(fmt.Sprintf("<%d bytes of binary data not collected>", len(value)))
		}
		c.Warn(c.AddYAML(objectPath("config/configmaps", cm.Namespace, cm.Name), cm))
	}
	return nil
}

func collectRBAC(ctx context.Context, c *Collection) error {
	lists := []struct {
		kind string
		list client.ObjectList
	}{
		{"ClusterRoles", &rbacv1.ClusterRoleList{}},
		{"ClusterRoleBindings", &rbacv1.ClusterRoleBindingList{}},
		{"Roles", &rbacv1.RoleList{}},
		{"RoleBindings", &rbacv1.RoleBindingList{}},
		{"ServiceAccounts", &corev1.ServiceAccountList{}},
	}
	failures := 0
	for _, l := range lists {
		if err := c.Env.Reader.List(ctx, l.list, client.MatchingLabels{managedByLabel: managedByOperator}, client.Limit(maxListItems)); err != nil {
			failures++
			c.Warn(fmt.Errorf("unable to list %s: %w", l.kind, err))
			continue
		}
		_ = meta.EachListItem(l.list, func(o runtime.Object) error {
			cleanTyped(o.(metav1.Object))
			return nil
		})
		items := reflect.ValueOf(l.list).Elem().FieldByName("Items").Interface()
		c.Warn(c.AddYAML("config/rbac/managed-"+strings.ToLower(l.kind)+".yaml", items))
	}
	if failures == len(lists) {
		return fmt.Errorf("unable to list RBAC resources")
	}

	collectOperatorRBAC(ctx, c)
	return nil
}

// collectOperatorRBAC collects the bindings of the Operator service account
// and the roles they grant.
func collectOperatorRBAC(ctx context.Context, c *Collection) {
	pod := c.Env.OperatorPod
	if pod == nil {
		return
	}
	saName := pod.Spec.ServiceAccountName
	if saName == "" {
		saName = "default"
	}
	isOperatorSA := func(subjects []rbacv1.Subject) bool {
		for _, s := range subjects {
			if s.Kind == rbacv1.ServiceAccountKind && s.Name == saName && s.Namespace == pod.Namespace {
				return true
			}
		}
		return false
	}
	getRole := func(ref rbacv1.RoleRef) (client.Object, error) {
		var role client.Object = &rbacv1.Role{}
		key := types.NamespacedName{Namespace: pod.Namespace, Name: ref.Name}
		if ref.Kind == "ClusterRole" {
			role = &rbacv1.ClusterRole{}
			key.Namespace = ""
		}
		if err := c.Env.Reader.Get(ctx, key, role); err != nil {
			return nil, fmt.Errorf("unable to get %s %s: %w", ref.Kind, ref.Name, err)
		}
		cleanTyped(role)
		return role, nil
	}

	operatorRBAC := map[string]any{"serviceAccount": pod.Namespace + "/" + saName}
	clusterRoleBindings := &rbacv1.ClusterRoleBindingList{}
	if err := c.Env.Reader.List(ctx, clusterRoleBindings); err != nil {
		c.Warn(fmt.Errorf("unable to list ClusterRoleBindings: %w", err))
	} else {
		var bindings []rbacv1.ClusterRoleBinding
		var roles []any
		for _, binding := range clusterRoleBindings.Items {
			if !isOperatorSA(binding.Subjects) {
				continue
			}
			cleanTyped(&binding)
			bindings = append(bindings, binding)
			if role, err := getRole(binding.RoleRef); err != nil {
				c.Warn(err)
			} else {
				roles = append(roles, role)
			}
		}
		operatorRBAC["clusterRoleBindings"] = bindings
		operatorRBAC["clusterRoles"] = roles
	}

	roleBindings := &rbacv1.RoleBindingList{}
	if err := c.Env.Reader.List(ctx, roleBindings, client.InNamespace(pod.Namespace)); err != nil {
		c.Warn(fmt.Errorf("unable to list RoleBindings: %w", err))
	} else {
		var bindings []rbacv1.RoleBinding
		var roles []any
		for _, binding := range roleBindings.Items {
			if !isOperatorSA(binding.Subjects) {
				continue
			}
			cleanTyped(&binding)
			bindings = append(bindings, binding)
			if role, err := getRole(binding.RoleRef); err != nil {
				c.Warn(err)
			} else {
				roles = append(roles, role)
			}
		}
		operatorRBAC["roleBindings"] = bindings
		operatorRBAC["roles"] = roles
	}
	c.Warn(c.AddYAML("config/rbac/operator-serviceaccount-rbac.yaml", operatorRBAC))
}

func collectWebhooks(ctx context.Context, c *Collection) error {
	add := func(kind string, webhook client.Object) {
		if webhook.GetLabels()[managedByLabel] == managedByOperator || strings.Contains(strings.ToLower(webhook.GetName()), "datadog") {
			cleanTyped(webhook)
			c.Warn(c.AddYAML(objectPath("config/webhooks/"+kind, "", webhook.GetName()), webhook))
		}
	}
	mutating := &admissionregistrationv1.MutatingWebhookConfigurationList{}
	mutatingErr := c.Env.Reader.List(ctx, mutating)
	if mutatingErr != nil {
		c.Warn(fmt.Errorf("unable to list MutatingWebhookConfigurations: %w", mutatingErr))
	}
	for i := range mutating.Items {
		for j := range mutating.Items[i].Webhooks {
			mutating.Items[i].Webhooks[j].ClientConfig.CABundle = nil
		}
		add("mutating", &mutating.Items[i])
	}
	validating := &admissionregistrationv1.ValidatingWebhookConfigurationList{}
	validatingErr := c.Env.Reader.List(ctx, validating)
	if validatingErr != nil {
		c.Warn(fmt.Errorf("unable to list ValidatingWebhookConfigurations: %w", validatingErr))
	}
	for i := range validating.Items {
		for j := range validating.Items[i].Webhooks {
			validating.Items[i].Webhooks[j].ClientConfig.CABundle = nil
		}
		add("validating", &validating.Items[i])
	}
	if mutatingErr != nil && validatingErr != nil {
		return fmt.Errorf("unable to list webhook configurations")
	}
	return nil
}

var crdGVK = schema.GroupVersionKind{Group: "apiextensions.k8s.io", Version: "v1", Kind: "CustomResourceDefinition"}

// crdSummary is an installed Datadog CRD in a flare. The schema isn't
// collected.
type crdSummary struct {
	Name           string            `json:"name"`
	Versions       []crdVersion      `json:"versions"`
	StoredVersions []string          `json:"storedVersions,omitempty"`
	Labels         map[string]string `json:"labels,omitempty"`
	Annotations    map[string]string `json:"annotations,omitempty"`
	Established    string            `json:"established,omitempty"`
}

type crdVersion struct {
	Name       string `json:"name"`
	Served     bool   `json:"served"`
	Storage    bool   `json:"storage"`
	Deprecated bool   `json:"deprecated,omitempty"`
}

func collectCRDs(ctx context.Context, c *Collection) error {
	// Unstructured, so that it works with clients whose scheme doesn't
	// register apiextensions, such as the kubectl plugin's.
	items, _, _, err := listUnstructured(ctx, c.Env.Reader, crdGVK)
	if err != nil {
		return fmt.Errorf("unable to list CustomResourceDefinitions: %w", err)
	}
	var summaries []crdSummary
	for i := range items {
		crd := apiextensionsv1.CustomResourceDefinition{}
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(items[i].Object, &crd); err != nil {
			c.Warn(fmt.Errorf("unable to read CustomResourceDefinition %s: %w", items[i].GetName(), err))
			continue
		}
		if crd.Spec.Group != datadogGroup {
			continue
		}
		cleanTyped(&crd)
		summary := crdSummary{
			Name:           crd.Name,
			StoredVersions: crd.Status.StoredVersions,
			Labels:         crd.Labels,
			Annotations:    crd.Annotations,
		}
		for _, v := range crd.Spec.Versions {
			summary.Versions = append(summary.Versions, crdVersion{Name: v.Name, Served: v.Served, Storage: v.Storage, Deprecated: v.Deprecated})
		}
		for _, cond := range crd.Status.Conditions {
			if cond.Type == apiextensionsv1.Established {
				summary.Established = string(cond.Status)
			}
		}
		summaries = append(summaries, summary)
	}
	return c.AddYAML("config/crds.yaml", summaries)
}

// helmLabels and helmAnnotations identify a Helm installation.
var (
	helmLabels      = []string{"helm.sh/chart", "app.kubernetes.io/managed-by", "app.kubernetes.io/instance", "app.kubernetes.io/version", "chart", "release", "heritage"}
	helmAnnotations = []string{"meta.helm.sh/release-name", "meta.helm.sh/release-namespace"}
)

// collectOperatorDeployment collects the Operator pod, its Deployment and its
// Helm chart metadata (not the values, which Helm keeps in a Secret).
func collectOperatorDeployment(ctx context.Context, c *Collection) error {
	pod := c.Env.OperatorPod
	if pod == nil {
		return fmt.Errorf("the Operator pod is unknown")
	}
	podCopy := pod.DeepCopy()
	cleanTyped(podCopy)
	c.Warn(c.AddYAML(objectPath("operator/pods", "", podCopy.Name), podCopy))

	deploy, err := operatorDeployment(ctx, c.Env.Reader, pod)
	if err != nil {
		return err
	}
	cleanTyped(deploy)
	c.Warn(c.AddYAML("operator/deployment.yaml", deploy))

	helm := map[string]string{}
	for _, key := range helmLabels {
		if value, ok := deploy.Labels[key]; ok {
			helm[key] = value
		}
	}
	for _, key := range helmAnnotations {
		if value, ok := deploy.Annotations[key]; ok {
			helm[key] = value
		}
	}
	installedWithHelm := deploy.Labels["app.kubernetes.io/managed-by"] == "Helm" || deploy.Annotations["meta.helm.sh/release-name"] != ""
	return c.AddYAML("operator/helm.yaml", map[string]any{
		"installedWithHelm": installedWithHelm,
		"metadata":          helm,
		"note":              "Helm keeps the release values in a Secret, which flares never collect. See operator/deployment.yaml for the flags and environment the values produced.",
	})
}

// operatorDeployment returns the Deployment of an Operator pod, through its
// ReplicaSet.
func operatorDeployment(ctx context.Context, reader client.Reader, pod *corev1.Pod) (*appsv1.Deployment, error) {
	for _, ref := range pod.OwnerReferences {
		if ref.Kind != "ReplicaSet" {
			continue
		}
		rs := &appsv1.ReplicaSet{}
		if err := reader.Get(ctx, types.NamespacedName{Namespace: pod.Namespace, Name: ref.Name}, rs); err != nil {
			return nil, fmt.Errorf("unable to get the Operator ReplicaSet: %w", err)
		}
		for _, rsRef := range rs.OwnerReferences {
			if rsRef.Kind != "Deployment" {
				continue
			}
			deploy := &appsv1.Deployment{}
			if err := reader.Get(ctx, types.NamespacedName{Namespace: pod.Namespace, Name: rsRef.Name}, deploy); err != nil {
				return nil, fmt.Errorf("unable to get the Operator Deployment: %w", err)
			}
			return deploy, nil
		}
	}
	return nil, fmt.Errorf("the Operator pod %s/%s isn't owned by a Deployment", pod.Namespace, pod.Name)
}
