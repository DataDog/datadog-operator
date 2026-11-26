// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package k8ssuite

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"testing"
	"time"

	"dario.cat/mergo"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner/parameters"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8syaml "k8s.io/apimachinery/pkg/util/yaml"
	corev1ac "k8s.io/client-go/applyconfigurations/core/v1"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/restmapper"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/test/e2e/common"
	"github.com/DataDog/datadog-operator/test/e2e/provisioners"
	"github.com/DataDog/datadog-operator/test/e2e/tests/utils"
)

// The subtests of TestGenericK8s swap the DatadogAgent and test workloads with
// the Kubernetes API instead of UpdateEnv. Every UpdateEnv is a full `pulumi up`
// over the whole stack (VM, fakeintake, kind, operator) and took 70-130s, i.e.
// most of the suite's runtime, to change one custom resource. Pulumi now only
// provisions the infrastructure and the operator, once, in SetupSuite.

const (
	// Names used by the e2e-framework's DatadogAgent component
	// (components/datadog/apps/dda), kept so the applied resources are unchanged.
	ddaCredentialsSecretName = "dda-datadog-credentials"
	imagePullSecretName      = "registry-credentials"

	scenarioFieldManager = "k8s-suite-e2e"
	scenarioTimeout      = 10 * time.Minute
	scenarioPollInterval = 2 * time.Second
)

var datadogAgentGVR = schema.GroupVersionResource{Group: "datadoghq.com", Version: "v2alpha1", Resource: "datadogagents"}

// scenario is the DatadogAgent and test workloads a subtest runs against.
type scenario struct {
	// ddaName is the DatadogAgent name, used when the manifest doesn't set one.
	ddaName string
	// ddaPath is the DatadogAgent manifest.
	ddaPath string
	// workloads are manifests applied next to the DatadogAgent. Workloads of the
	// previous scenario that are not listed are deleted.
	workloads []string
}

type objectRef struct {
	gvr       schema.GroupVersionResource
	namespace string
	name      string
}

// scenarioState holds the clients and what the previous scenario applied.
type scenarioState struct {
	dynamic   dynamic.Interface
	mapper    meta.RESTMapper
	workloads map[objectRef]struct{}
}

// applyScenario replaces the running DatadogAgent and test workloads with the
// ones of sc. It mirrors what UpdateEnv used to do in two steps (remove the
// previous DatadogAgent, then apply the new one), without Pulumi:
//
//  1. delete the previous DatadogAgent, wait until it is gone (its finalizer
//     deletes its DatadogAgentInternals) and until its pods are gone. A new
//     agent applied while the previous agent pods are still terminating can get
//     stuck on resources they hold, such as host ports;
//  2. delete the previous workloads that sc doesn't list and apply sc's;
//  3. apply the DatadogAgent, with the settings the e2e-framework used to add
//     (cluster name, credentials, fakeintake URLs, image pull secret).
func (s *k8sSuite) applyScenario(t *testing.T, sc scenario) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), scenarioTimeout)
	defer cancel()

	state := s.scenarioState(t)

	start := time.Now()
	require.NoError(t, s.applyDDASecrets(ctx), "apply DatadogAgent secrets")

	require.NoError(t, utils.DeleteAllDatadogAgents(ctx, s.Env().KubernetesCluster.KubernetesClient.K8sConfig, common.NamespaceName),
		"delete the previous DatadogAgent")
	require.NoError(t, s.waitForAgentPodsDeletion(ctx), "wait for the previous agent pods to be deleted")
	t.Logf("Previous DatadogAgent removed in %s", time.Since(start).Round(time.Second))

	workloads := map[objectRef]*unstructured.Unstructured{}
	for _, path := range sc.workloads {
		objs, err := decodeManifest(path)
		require.NoError(t, err, "decode workload %s", path)
		for _, obj := range objs {
			ref, err := state.refFor(obj)
			require.NoError(t, err, "resolve workload %s", path)
			workloads[ref] = obj
		}
	}
	for ref := range state.workloads {
		if _, keep := workloads[ref]; keep {
			continue
		}
		err := state.dynamic.Resource(ref.gvr).Namespace(ref.namespace).Delete(ctx, ref.name, metav1.DeleteOptions{})
		if err != nil && !apierrors.IsNotFound(err) {
			require.NoError(t, err, "delete workload %s/%s", ref.namespace, ref.name)
		}
		delete(state.workloads, ref)
	}
	for ref, obj := range workloads {
		_, err := state.dynamic.Resource(ref.gvr).Namespace(ref.namespace).
			Apply(ctx, ref.name, obj, metav1.ApplyOptions{FieldManager: scenarioFieldManager, Force: true})
		require.NoError(t, err, "apply workload %s/%s", ref.namespace, ref.name)
		state.workloads[ref] = struct{}{}
	}

	fakeIntakeURL := ""
	if s.Env().FakeIntake != nil {
		fakeIntakeURL = s.Env().FakeIntake.URL
	}
	dda, err := buildDatadogAgent(sc, s.Env().KubernetesCluster.ClusterName, fakeIntakeURL, s.usesImagePullSecret())
	require.NoError(t, err, "build DatadogAgent from %s", sc.ddaPath)
	// Set before applying: a failed Apply may still have created the resource,
	// and the suite cleanup must then delete it before the stack destroy.
	s.ddaApplied = true
	_, err = state.dynamic.Resource(datadogAgentGVR).Namespace(dda.GetNamespace()).
		Apply(ctx, dda.GetName(), dda, metav1.ApplyOptions{FieldManager: scenarioFieldManager, Force: true})
	require.NoError(t, err, "apply DatadogAgent %s", dda.GetName())

	t.Logf("Scenario %s applied in %s", dda.GetName(), time.Since(start).Round(time.Second))
}

func (s *k8sSuite) scenarioState(t *testing.T) *scenarioState {
	if s.scenario != nil {
		return s.scenario
	}
	cfg := s.Env().KubernetesCluster.KubernetesClient.K8sConfig
	dyn, err := dynamic.NewForConfig(cfg)
	require.NoError(t, err, "create dynamic client")
	disco, err := discovery.NewDiscoveryClientForConfig(cfg)
	require.NoError(t, err, "create discovery client")
	s.scenario = &scenarioState{
		dynamic:   dyn,
		mapper:    restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(disco)),
		workloads: map[objectRef]struct{}{},
	}
	return s.scenario
}

func (st *scenarioState) refFor(obj *unstructured.Unstructured) (objectRef, error) {
	gvk := obj.GroupVersionKind()
	mapping, err := st.mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return objectRef{}, err
	}
	ns := obj.GetNamespace()
	if ns == "" && mapping.Scope.Name() == meta.RESTScopeNameNamespace {
		ns = common.NamespaceName
		obj.SetNamespace(ns)
	}
	return objectRef{gvr: mapping.Resource, namespace: ns, name: obj.GetName()}, nil
}

// applyDDASecrets applies the credentials and image pull secrets the
// DatadogAgent references, with the values the e2e-framework uses.
func (s *k8sSuite) applyDDASecrets(ctx context.Context) error {
	secrets := s.Env().KubernetesCluster.Client().CoreV1().Secrets(common.NamespaceName)
	opts := metav1.ApplyOptions{FieldManager: scenarioFieldManager, Force: true}

	apiKey, err := runner.GetProfile().SecretStore().Get(parameters.APIKey)
	if err != nil {
		return fmt.Errorf("get API key: %w", err)
	}
	appKey, err := runner.GetProfile().SecretStore().GetWithDefault(parameters.APPKey, "")
	if err != nil {
		return fmt.Errorf("get APP key: %w", err)
	}
	credentials := corev1ac.Secret(ddaCredentialsSecretName, common.NamespaceName).
		WithStringData(map[string]string{"api-key": apiKey, "app-key": appKey})
	if _, err := secrets.Apply(ctx, credentials, opts); err != nil {
		return fmt.Errorf("apply %s: %w", ddaCredentialsSecretName, err)
	}

	if !s.usesImagePullSecret() {
		return nil
	}
	auth := map[string]string{
		"username": provisioners.ImagePullUsername,
		"password": common.ImgPullPassword,
		"auth":     base64.StdEncoding.EncodeToString([]byte(provisioners.ImagePullUsername + ":" + common.ImgPullPassword)),
	}
	dockerConfig, err := json.Marshal(map[string]map[string]map[string]string{"auths": {provisioners.ImagePullRegistry: auth}})
	if err != nil {
		return err
	}
	pullSecret := corev1ac.Secret(imagePullSecretName, common.NamespaceName).
		WithType(corev1.SecretTypeDockerConfigJson).
		WithData(map[string][]byte{corev1.DockerConfigJsonKey: dockerConfig})
	if _, err := secrets.Apply(ctx, pullSecret, opts); err != nil {
		return fmt.Errorf("apply %s: %w", imagePullSecretName, err)
	}
	return nil
}

// usesImagePullSecret reports whether the provisioner configured an image pull
// registry, in which case the e2e-framework added a pull secret to the DatadogAgent.
func (s *k8sSuite) usesImagePullSecret() bool {
	return provisioners.UsesKindVM(s.local) && common.ImgPullPassword != ""
}

// waitForAgentPodsDeletion waits until no pod of any DatadogAgent is left.
func (s *k8sSuite) waitForAgentPodsDeletion(ctx context.Context) error {
	pods := s.Env().KubernetesCluster.Client().CoreV1().Pods(common.NamespaceName)
	for {
		list, err := pods.List(ctx, metav1.ListOptions{LabelSelector: apicommon.AgentDeploymentNameLabelKey})
		if err != nil {
			return err
		}
		if len(list.Items) == 0 {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("%d agent pods left (e.g. %s): %w", len(list.Items), list.Items[0].Name, ctx.Err())
		case <-time.After(scenarioPollInterval):
		}
	}
}

// buildDatadogAgent reads sc's DatadogAgent manifest and adds to it what the
// e2e-framework's DatadogAgent component (components/datadog/apps/dda) adds,
// with the same mergo calls so the applied resource doesn't change. An empty
// fakeIntakeURL adds no fakeintake settings.
func buildDatadogAgent(sc scenario, clusterName, fakeIntakeURL string, imagePullSecret bool) (*unstructured.Unstructured, error) {
	objs, err := decodeManifest(sc.ddaPath)
	if err != nil {
		return nil, err
	}
	if len(objs) != 1 {
		return nil, fmt.Errorf("expected one DatadogAgent in %s, got %d objects", sc.ddaPath, len(objs))
	}
	state := objs[0].Object

	defaultMetadata := map[string]any{"name": sc.ddaName, "namespace": common.NamespaceName}
	if state["metadata"] == nil {
		state["metadata"] = defaultMetadata
	} else {
		stateMetadata := state["metadata"].(map[string]any)
		if err := mergo.Merge(&stateMetadata, defaultMetadata); err != nil {
			return nil, err
		}
	}

	spec, ok := state["spec"].(map[string]any)
	if !ok {
		spec = map[string]any{}
		state["spec"] = spec
	}

	defaultGlobal := map[string]any{
		"clusterName": clusterName,
		"credentials": map[string]any{
			"apiSecret": map[string]any{"secretName": ddaCredentialsSecretName, "keyName": "api-key"},
			"appSecret": map[string]any{"secretName": ddaCredentialsSecretName, "keyName": "app-key"},
		},
	}
	if spec["global"] == nil {
		spec["global"] = defaultGlobal
	} else {
		stateGlobal := spec["global"].(map[string]any)
		if err := mergo.Map(&stateGlobal, defaultGlobal); err != nil {
			return nil, err
		}
	}

	overrides := []map[string]any{}
	if fakeIntakeURL != "" {
		env := fakeIntakeEnvVars(fakeIntakeURL)
		overrides = append(overrides, perComponent(func() any { return map[string]any{"env": env} }))
	}
	if imagePullSecret {
		overrides = append(overrides, perComponent(func() any {
			return map[string]any{"image": map[string]any{
				"pullSecrets": []map[string]any{{"name": imagePullSecretName}},
			}}
		}))
	}
	for _, override := range overrides {
		if spec["override"] == nil {
			spec["override"] = override
			continue
		}
		stateOverride := spec["override"].(map[string]any)
		if err := mergo.Map(&stateOverride, override); err != nil {
			return nil, err
		}
	}

	// Normalize the typed slices and maps added above into JSON values.
	raw, err := json.Marshal(state)
	if err != nil {
		return nil, err
	}
	dda := &unstructured.Unstructured{}
	if err := dda.UnmarshalJSON(raw); err != nil {
		return nil, err
	}
	return dda, nil
}

func perComponent(value func() any) map[string]any {
	return map[string]any{
		"nodeAgent":           value(),
		"clusterAgent":        value(),
		"clusterChecksRunner": value(),
	}
}

func fakeIntakeEnvVars(url string) []map[string]any {
	env := []map[string]any{}
	for _, name := range []string{
		"DD_DD_URL",
		"DD_PROCESS_CONFIG_PROCESS_DD_URL",
		"DD_APM_DD_URL",
		"DD_LOGS_CONFIG_LOGS_DD_URL",
	} {
		env = append(env, map[string]any{"name": name, "value": url})
	}
	env = append(env, map[string]any{"name": "DD_LOGS_CONFIG_USE_HTTP", "value": "true"})
	env = append(env, map[string]any{"name": "DD_SERVICE_DISCOVERY_FORWARDER_LOGS_DD_URL", "value": url})
	env = append(env, map[string]any{"name": "DD_SKIP_SSL_VALIDATION", "value": "true"})
	env = append(env, map[string]any{"name": "DD_REMOTE_CONFIGURATION_NO_TLS_VALIDATION", "value": "true"})
	return env
}

// decodeManifest returns the objects of a (multi-document) YAML manifest.
func decodeManifest(path string) ([]*unstructured.Unstructured, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	decoder := k8syaml.NewYAMLOrJSONDecoder(bytes.NewReader(data), 4096)
	var objs []*unstructured.Unstructured
	for {
		obj := &unstructured.Unstructured{}
		if err := decoder.Decode(&obj.Object); err != nil {
			if errors.Is(err, io.EOF) {
				return objs, nil
			}
			return nil, fmt.Errorf("decode %s: %w", path, err)
		}
		if len(obj.Object) == 0 {
			continue
		}
		objs = append(objs, obj)
	}
}
