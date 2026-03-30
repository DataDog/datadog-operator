// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package render

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	goruntime "runtime"

	edsdatadoghqv1alpha1 "github.com/DataDog/extendeddaemonset/api/v1alpha1"
	"github.com/go-logr/logr"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	policyv1 "k8s.io/api/policy/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	apiregistrationv1 "k8s.io/kube-aggregator/pkg/apis/apiregistration/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/yaml"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	datadogagent "github.com/DataDog/datadog-operator/internal/controller/datadogagent"
	componentagent "github.com/DataDog/datadog-operator/internal/controller/datadogagent/component/agent"
	datadogagentinternal "github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

// RenderOptions controls which reconciliation path and features are used during rendering.
type RenderOptions struct {
	DatadogAgentInternalEnabled bool
	DatadogAgentProfileEnabled  bool
	SupportExtendedDaemonset    bool
	SupportCilium               bool
}

// Run is the entry point for the render subcommand.
// It parses its own flags from the provided args and writes rendered manifests to stdout.
func Run(args []string) error {
	fs := flag.NewFlagSet("render", flag.ExitOnError)
	filename := fs.String("f", "", "Path to DatadogAgent YAML file (required)")
	output := fs.String("o", "yaml", "Output format: yaml or json")
	namespace := fs.String("n", "", "Override namespace for generated resources")
	datadogAgentInternalEnabled := fs.Bool("datadogAgentInternalEnabled", true, "Enable DatadogAgentInternal reconciliation (v3 path)")
	datadogAgentProfileEnabled := fs.Bool("datadogAgentProfileEnabled", false, "Enable DatadogAgentProfile support")
	supportExtendedDaemonset := fs.Bool("supportExtendedDaemonset", false, "Use ExtendedDaemonSet instead of DaemonSet")
	supportCilium := fs.Bool("supportCilium", false, "Enable Cilium CNI support")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: operator render [flags]\n\nRender Kubernetes manifests from a DatadogAgent resource without a cluster.\n\nFlags:\n")
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		return err
	}

	if *filename == "" {
		fs.Usage()
		return fmt.Errorf("flag -f is required")
	}

	dda, err := loadDatadogAgent(*filename, *namespace)
	if err != nil {
		return err
	}

	s := newScheme()

	renderOpts := RenderOptions{
		DatadogAgentInternalEnabled: *datadogAgentInternalEnabled,
		DatadogAgentProfileEnabled:  *datadogAgentProfileEnabled,
		SupportExtendedDaemonset:    *supportExtendedDaemonset,
		SupportCilium:               *supportCilium,
	}

	objects, err := RenderManifests(dda, s, renderOpts)
	if err != nil {
		return fmt.Errorf("render failed: %w", err)
	}

	return serializeObjects(objects, *output, s, os.Stdout)
}

// loadDatadogAgent reads and parses a DatadogAgent YAML file.
func loadDatadogAgent(path string, namespaceOverride string) (*v2alpha1.DatadogAgent, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read file %s: %w", path, err)
	}

	dda := &v2alpha1.DatadogAgent{}
	if err := yaml.Unmarshal(data, dda); err != nil {
		return nil, fmt.Errorf("failed to parse DatadogAgent YAML: %w", err)
	}

	if dda.APIVersion == "" {
		dda.APIVersion = "datadoghq.com/v2alpha1"
	}
	if dda.Kind == "" {
		dda.Kind = "DatadogAgent"
	}
	if namespaceOverride != "" {
		dda.Namespace = namespaceOverride
	}
	if dda.Namespace == "" {
		dda.Namespace = "datadog"
	}
	if dda.Name == "" {
		return nil, fmt.Errorf("DatadogAgent must have metadata.name set")
	}

	return dda, nil
}

// RenderManifests runs the reconciler offline against a DatadogAgent and returns
// all Kubernetes resources that would be generated.
// If s is nil, a default scheme is created.
func RenderManifests(dda *v2alpha1.DatadogAgent, s *runtime.Scheme, renderOpts RenderOptions) ([]client.Object, error) {
	logf.SetLogger(zap.New(zap.WriteTo(os.Stderr), zap.UseDevMode(false)))
	logger := logf.Log.WithName("render")

	if s == nil {
		s = newScheme()
	}

	clientBuilder := fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(dda).
		WithStatusSubresource(&v2alpha1.DatadogAgent{}, &appsv1.DaemonSet{})

	// When DDAI is enabled, the field manager needs the CRD in the fake client
	if renderOpts.DatadogAgentInternalEnabled {
		crd, err := loadDDAICRD(s)
		if err != nil {
			return nil, fmt.Errorf("failed to load DDAI CRD: %w", err)
		}
		clientBuilder = clientBuilder.
			WithObjects(crd).
			WithStatusSubresource(&v1alpha1.DatadogAgentInternal{})
	}

	fakeClient := clientBuilder.Build()

	eventBroadcaster := record.NewBroadcaster()
	recorder := eventBroadcaster.NewRecorder(s, corev1.EventSource{Component: "render"})

	opts := datadogagent.ReconcilerOptions{
		OperatorMetricsEnabled:      false,
		IntrospectionEnabled:        false,
		DatadogAgentProfileEnabled:  renderOpts.DatadogAgentProfileEnabled,
		DatadogAgentInternalEnabled: renderOpts.DatadogAgentInternalEnabled,
		SupportCilium:               renderOpts.SupportCilium,
		ExtendedDaemonsetOptions: componentagent.ExtendedDaemonsetOptions{
			Enabled: renderOpts.SupportExtendedDaemonset,
		},
	}

	r, err := datadogagent.NewReconciler(opts, fakeClient, kubernetes.PlatformInfo{}, s, logger, recorder, noopForwarders{})
	if err != nil {
		return nil, fmt.Errorf("failed to create reconciler: %w", err)
	}

	ctx := context.Background()

	// First reconcile: adds the finalizer and requeues
	if _, err = r.Reconcile(ctx, dda); err != nil {
		return nil, fmt.Errorf("first reconcile (finalizer) failed: %w", err)
	}

	// Re-read the DDA from the fake client (now has the finalizer)
	updatedDDA := &v2alpha1.DatadogAgent{}
	if err = fakeClient.Get(ctx, client.ObjectKeyFromObject(dda), updatedDDA); err != nil {
		return nil, fmt.Errorf("failed to re-read DDA after finalizer: %w", err)
	}

	// Second reconcile: generates resources (v2) or DDAI objects (v3)
	if _, err = r.Reconcile(ctx, updatedDDA); err != nil {
		return nil, fmt.Errorf("reconcile failed: %w", err)
	}

	// When DDAI is enabled, the DDA reconciler creates DDAI objects instead of
	// workload resources. We must run the DDAI reconciler to produce the final resources.
	if renderOpts.DatadogAgentInternalEnabled {
		if err = reconcileDDAIs(ctx, fakeClient, s, recorder, renderOpts); err != nil {
			return nil, fmt.Errorf("DDAI reconcile failed: %w", err)
		}
	}

	return extractAllObjects(ctx, fakeClient, logger)
}

// reconcileDDAIs lists DDAI objects created by the DDA reconciler and runs the
// DDAI reconciler on each to produce the actual workload resources.
func reconcileDDAIs(ctx context.Context, fakeClient client.Client, s *runtime.Scheme, recorder record.EventRecorder, renderOpts RenderOptions) error {
	ddaiOpts := datadogagentinternal.ReconcilerOptions{
		OperatorMetricsEnabled: false,
		SupportCilium:          renderOpts.SupportCilium,
		ExtendedDaemonsetOptions: componentagent.ExtendedDaemonsetOptions{
			Enabled: renderOpts.SupportExtendedDaemonset,
		},
	}

	ri, err := datadogagentinternal.NewReconciler(ddaiOpts, fakeClient, kubernetes.PlatformInfo{}, s, recorder, noopForwarders{})
	if err != nil {
		return fmt.Errorf("failed to create DDAI reconciler: %w", err)
	}

	ddais := &v1alpha1.DatadogAgentInternalList{}
	if err = fakeClient.List(ctx, ddais); err != nil {
		return fmt.Errorf("failed to list DDAI objects: %w", err)
	}

	for i := range ddais.Items {
		ddai := &ddais.Items[i]
		// The DDAI was created with a finalizer by generateDDAIFromDDA,
		// so the reconciler proceeds directly to resource creation.
		if _, err = ri.Reconcile(ctx, ddai); err != nil {
			return fmt.Errorf("failed to reconcile DDAI %s/%s: %w", ddai.Namespace, ddai.Name, err)
		}
	}

	return nil
}

// loadDDAICRD reads the DDAI CRD YAML from the config directory and decodes it.
// The CRD is needed by the field manager when DatadogAgentInternalEnabled is true.
func loadDDAICRD(s *runtime.Scheme) (*apiextensionsv1.CustomResourceDefinition, error) {
	_, filename, _, ok := goruntime.Caller(0)
	if !ok {
		return nil, fmt.Errorf("unable to get caller")
	}
	path := filepath.Join(filepath.Dir(filename), "..", "..", "..", "..", "config", "crd", "bases", "v1", "datadoghq.com_datadogagentinternals.yaml")

	body, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read DDAI CRD file: %w", err)
	}

	codecs := serializer.NewCodecFactory(s)
	decoder := codecs.UniversalDeserializer()
	obj, _, err := decoder.Decode(body, nil, &apiextensionsv1.CustomResourceDefinition{})
	if err != nil {
		return nil, fmt.Errorf("failed to decode DDAI CRD: %w", err)
	}

	crd, ok := obj.(*apiextensionsv1.CustomResourceDefinition)
	if !ok {
		return nil, fmt.Errorf("decoded object is not a CustomResourceDefinition")
	}

	return crd, nil
}

// extractAllObjects lists all Kubernetes resources from the fake client,
// filtering out the DDA input object.
func extractAllObjects(ctx context.Context, c client.Client, logger logr.Logger) ([]client.Object, error) {
	var result []client.Object

	lists := []client.ObjectList{
		&appsv1.DaemonSetList{},
		&appsv1.DeploymentList{},
		&corev1.ServiceAccountList{},
		&corev1.ServiceList{},
		&corev1.SecretList{},
		&corev1.ConfigMapList{},
		&rbacv1.ClusterRoleList{},
		&rbacv1.ClusterRoleBindingList{},
		&rbacv1.RoleList{},
		&rbacv1.RoleBindingList{},
		&networkingv1.NetworkPolicyList{},
		&apiregistrationv1.APIServiceList{},
		&admissionregistrationv1.MutatingWebhookConfigurationList{},
		&admissionregistrationv1.ValidatingWebhookConfigurationList{},
		&policyv1.PodDisruptionBudgetList{},
		&v1alpha1.DatadogAgentInternalList{},
	}

	for _, list := range lists {
		if err := c.List(ctx, list); err != nil {
			logger.V(1).Info("skipping resource type during extraction", "error", err)
			continue
		}
		items, err := meta.ExtractList(list)
		if err != nil {
			continue
		}
		for _, item := range items {
			obj, ok := item.(client.Object)
			if !ok {
				continue
			}
			// Skip the DDA input object
			if _, isDDA := obj.(*v2alpha1.DatadogAgent); isDDA {
				continue
			}
			cleanObject(obj)
			result = append(result, obj)
		}
	}
	return result, nil
}

// newScheme creates a runtime.Scheme with all types needed for reconciliation.
func newScheme() *runtime.Scheme {
	s := scheme.Scheme
	s.AddKnownTypes(edsdatadoghqv1alpha1.GroupVersion, &edsdatadoghqv1alpha1.ExtendedDaemonSet{})
	s.AddKnownTypes(v1alpha1.GroupVersion, &v1alpha1.DatadogAgentProfile{})
	s.AddKnownTypes(v1alpha1.GroupVersion, &v1alpha1.DatadogAgentProfileList{})
	s.AddKnownTypes(v1alpha1.GroupVersion, &v1alpha1.DatadogAgentInternal{})
	s.AddKnownTypes(v1alpha1.GroupVersion, &v1alpha1.DatadogAgentInternalList{})
	s.AddKnownTypes(v2alpha1.GroupVersion, &v2alpha1.DatadogAgent{})
	s.AddKnownTypes(v2alpha1.GroupVersion, &v2alpha1.DatadogAgentList{})
	s.AddKnownTypes(apiregistrationv1.SchemeGroupVersion, &apiregistrationv1.APIService{})
	s.AddKnownTypes(apiregistrationv1.SchemeGroupVersion, &apiregistrationv1.APIServiceList{})
	s.AddKnownTypes(apiextensionsv1.SchemeGroupVersion, &apiextensionsv1.CustomResourceDefinition{})
	return s
}
