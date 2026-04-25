# KSM Core Advanced Options — Bucket A Implementation Plan

> **Ticket:** [CONTP-1569](https://datadoghq.atlassian.net/browse/CONTP-1569) — sub-task of epic [CONTP-1446](https://datadoghq.atlassian.net/browse/CONTP-1446) (Operator ↔ Helm chart parity).
>
> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Expose 5 Helm-parity fields on `KubeStateMetricsCoreFeatureConfig` so users can tune KSM collection and tagging without dropping to a full custom `Conf` ConfigMap.

**Architecture:** The KSM-Core feature is implemented in `internal/controller/datadogagent/feature/kubernetesstatecore/`. The feature currently threads a small `collectorOptions` struct from `Configure()` into `buildKSMCoreConfigMap()` (YAML generation) and `getRBACPolicyRules()` (RBAC). We extend both the CRD surface and `collectorOptions` with 5 additive fields; each field independently controls either the rendered check-instance YAML (`tags`, `labelsAsTags`, `annotationsAsTags`), the check-instance YAML + ClusterRole RBAC (`collectSecretMetrics`, `collectConfigMaps`), or both. All fields are additive, `*bool` / `omitempty`, and default to the operator's current behavior. Defaults exactly match Helm: `collectSecretMetrics` and `collectConfigMaps` default to `true`; `labelsAsTags`, `annotationsAsTags`, and `tags` default to empty.

**Tech Stack:** Go 1.x · kubebuilder v2 API types · operator-sdk / controller-runtime · `sigs.k8s.io/controller-tools` codegen · Kubernetes RBAC (`rbacv1.PolicyRule`) · YAML generation via `bytes.Buffer` + `gopkg.in/yaml.v3` · `testify` + table-driven tests.

---

## Scope

**In scope (Bucket A — 5 fields, 1 PR):**

| Field (CRD JSON) | Type | Default | Touches |
|---|---|---|---|
| `tags` | `[]string` | `nil` | configmap.go |
| `labelsAsTags` | `map[string]map[string]string` | `nil` | configmap.go |
| `annotationsAsTags` | `map[string]map[string]string` | `nil` | configmap.go |
| `collectSecretMetrics` | `*bool` | `true` | configmap.go + rbac.go |
| `collectConfigMaps` | `*bool` | `true` | configmap.go + rbac.go |

**Explicitly deferred (separate tickets):**

- `namespaces` — RBAC topology change (ClusterRole → per-namespace Role+RoleBinding). Own ticket.
- `collectVpaMetrics` / `collectCrdMetrics` / `collectApiServicesMetrics` — need precedence-policy decision vs. existing version/platform gates.

**Reference:** Helm chart source of truth at `../helm-charts/charts/datadog/templates/_kubernetes_state_core_config.yaml` and `../helm-charts/charts/datadog/templates/kube-state-metrics-core-rbac.yaml`. Helm values shape verified in `../helm-charts/charts/datadog/values.yaml`.

---

## Prerequisites

- [ ] **P1: Create a dedicated feature branch off `main`.**

  The current branch `minyi/cons-8253-fix` is unrelated work for CONS-8253 — commits for this ticket belong on a new branch.

  ```bash
  cd /Users/minyi.zhu/Desktop/DataDog/datadog-operator
  git fetch origin
  git checkout -b minyi/contp-1569-ksm-bucket-a origin/main
  ```

- [ ] **P2: Confirm baseline tests pass before any change.**

  ```bash
  go test ./internal/controller/datadogagent/feature/kubernetesstatecore/... -v
  ```
  Expected: PASS. If baseline fails, stop and report — don't layer changes on a red tree.

---

## File Structure

**Modified:**
- `api/datadoghq/v2alpha1/datadogagent_types.go` (lines 907–932) — add 5 fields to `KubeStateMetricsCoreFeatureConfig`.
- `internal/controller/datadogagent/feature/kubernetesstatecore/feature.go` — add 5 fields to `collectorOptions`; plumb from `ddaSpec.Features.KubeStateMetricsCore.*` in `Configure()`.
- `internal/controller/datadogagent/feature/kubernetesstatecore/configmap.go` — extend `ksmCheckConfig()` to conditionally emit `secrets` / `configmaps` collectors and new `labels_as_tags`, `annotations_as_tags`, `tags` sections.
- `internal/controller/datadogagent/feature/kubernetesstatecore/rbac.go` — conditionally drop `ConfigMapsResource` / `SecretsResource` from the core-API rule.
- `pkg/testutils/builder.go` — add 5 `WithKSM*` builder helpers (mirroring existing `WithKSMEnabled` / `WithKSMCustomConf`).

**Test files extended (no new files):**
- `internal/controller/datadogagent/feature/kubernetesstatecore/configmap_yaml_test.go`
- `internal/controller/datadogagent/feature/kubernetesstatecore/configmap_test.go`
- `internal/controller/datadogagent/feature/kubernetesstatecore/rbac_test.go`
- `internal/controller/datadogagent/feature/kubernetesstatecore/feature_test.go`

**Regenerated (do not hand-edit):**
- `api/datadoghq/v2alpha1/zz_generated.deepcopy.go`
- `api/datadoghq/v2alpha1/zz_generated.openapi.go`
- `config/crd/bases/v1/datadoghq.com_datadogagents.yaml`
- `docs/configuration.v2alpha1.md`

---

## Task 1 — Add CRD fields

**Files:**
- Modify: `api/datadoghq/v2alpha1/datadogagent_types.go:907-932`

- [ ] **Step 1.1: Replace the `KubeStateMetricsCoreFeatureConfig` struct.**

  Open `api/datadoghq/v2alpha1/datadogagent_types.go`. Find the struct starting at line 911. Replace the whole struct (lines 907–932) with:

  ```go
  // KubeStateMetricsCoreFeatureConfig contains the Kube State Metrics Core check feature configuration.
  // The Kube State Metrics Core check runs in the Cluster Agent (or Cluster Check Runners).
  // See also: https://docs.datadoghq.com/integrations/kubernetes_state_core
  // +k8s:openapi-gen=true
  type KubeStateMetricsCoreFeatureConfig struct {
  	// Enabled enables Kube State Metrics Core.
  	// Default: true
  	// +optional
  	Enabled *bool `json:"enabled,omitempty"`

  	// Conf overrides the configuration for the default Kubernetes State Metrics Core check.
  	// This must point to a ConfigMap containing a valid cluster check configuration.
  	// +doc-gen:truncate
  	// +optional
  	Conf *CustomConfig `json:"conf,omitempty"`

  	// `CollectCrMetrics` defines custom resources for the kube-state-metrics core check to collect.
  	//
  	// The datadog agent uses the same logic as upstream `kube-state-metrics`. So is its configuration.
  	// The exact structure and existing fields of each item in this list can be found in:
  	// https://github.com/kubernetes/kube-state-metrics/blob/main/docs/metrics/extend/customresourcestate-metrics.md
  	//
  	// +optional
  	// +listType=atomic
  	CollectCrMetrics []Resource `json:"collectCrMetrics,omitempty"`

  	// CollectSecretMetrics enables collection of metrics on Secrets.
  	// When false, the `secrets` collector and the RBAC permission to list/watch Secrets are omitted.
  	// Default: true
  	// +optional
  	CollectSecretMetrics *bool `json:"collectSecretMetrics,omitempty"`

  	// CollectConfigMaps enables collection of metrics on ConfigMaps.
  	// When false, the `configmaps` collector and the RBAC permission to list/watch ConfigMaps are omitted.
  	// Default: true
  	// +optional
  	CollectConfigMaps *bool `json:"collectConfigMaps,omitempty"`

  	// LabelsAsTags maps Kubernetes labels to Datadog tags, scoped to the KSM check.
  	// Outer key is the Kubernetes resource kind (e.g. "pod", "node", "deployment");
  	// inner map is label name -> Datadog tag name.
  	// Example:
  	//   labelsAsTags:
  	//     pod:
  	//       app: app
  	//     node:
  	//       zone: zone
  	// Note: the top-level `global.kubernetesResourcesLabelsAsTags` configures this at the agent level
  	// via environment variables. LabelsAsTags here writes into the KSM check instance config and
  	// applies only to KSM metrics.
  	// +optional
  	LabelsAsTags map[string]map[string]string `json:"labelsAsTags,omitempty"`

  	// AnnotationsAsTags maps Kubernetes annotations to Datadog tags, scoped to the KSM check.
  	// Outer key is the Kubernetes resource kind; inner map is annotation name -> Datadog tag name.
  	// Annotation names must match the transformation done by kube-state-metrics
  	// (e.g. `tags.datadoghq.com/version` becomes `tags_datadoghq_com_version`).
  	// +optional
  	AnnotationsAsTags map[string]map[string]string `json:"annotationsAsTags,omitempty"`

  	// Tags is a list of static tags applied to all KSM metrics.
  	// Format: `key:value`.
  	// +optional
  	// +listType=atomic
  	Tags []string `json:"tags,omitempty"`
  }
  ```

- [ ] **Step 1.2: Regenerate deepcopy, OpenAPI, and CRD manifest.**

  ```bash
  cd /Users/minyi.zhu/Desktop/DataDog/datadog-operator
  make generate && make manifests
  ```
  Expected: no errors. Tracked files `api/datadoghq/v2alpha1/zz_generated.deepcopy.go`, `api/datadoghq/v2alpha1/zz_generated.openapi.go`, and `config/crd/bases/v1/datadoghq.com_datadogagents.yaml` are modified; `docs/configuration.v2alpha1.md` may also update.

- [ ] **Step 1.3: Verify the CRD surface compiles.**

  ```bash
  go build ./...
  ```
  Expected: success. (The feature code does not yet read the new fields, so no behavior changes.)

- [ ] **Step 1.4: Commit.**

  ```bash
  git add api/datadoghq/v2alpha1/datadogagent_types.go \
          api/datadoghq/v2alpha1/zz_generated.deepcopy.go \
          api/datadoghq/v2alpha1/zz_generated.openapi.go \
          config/crd/bases/v1/datadoghq.com_datadogagents.yaml \
          docs/configuration.v2alpha1.md
  git commit -m "[CONTP-1569] Add 5 KSM core CRD fields for Helm parity

  Adds CollectSecretMetrics, CollectConfigMaps, LabelsAsTags, AnnotationsAsTags,
  and Tags to KubeStateMetricsCoreFeatureConfig. Schema-only change; feature code
  still uses prior defaults until subsequent commits wire each field through."
  ```

---

## Task 2 — Extend `collectorOptions` and plumb from `Configure`

**Files:**
- Modify: `internal/controller/datadogagent/feature/kubernetesstatecore/feature.go:181-187` (struct `collectorOptions`)
- Modify: `internal/controller/datadogagent/feature/kubernetesstatecore/feature.go:83-179` (`Configure` method)

- [ ] **Step 2.1: Extend the `collectorOptions` struct.**

  Replace the `collectorOptions` struct (feature.go lines 181–187) with:

  ```go
  type collectorOptions struct {
  	enableVPA                 bool
  	enableAPIService          bool
  	enableCRD                 bool
  	enableControllerRevisions bool
  	collectSecrets            bool
  	collectConfigMaps         bool
  	customResources           []v2alpha1.Resource
  	labelsAsTags              map[string]map[string]string
  	annotationsAsTags         map[string]map[string]string
  	tags                      []string
  }
  ```

- [ ] **Step 2.2: Add five fields to the `ksmFeature` struct.**

  Find `type ksmFeature struct` (starts at feature.go:49). Immediately after the `collectCrMetrics []v2alpha1.Resource` line, add:

  ```go
  	collectSecrets    bool
  	collectConfigMapsMetrics bool
  	labelsAsTags      map[string]map[string]string
  	annotationsAsTags map[string]map[string]string
  	tags              []string
  ```

  (Renamed to `collectConfigMapsMetrics` on the struct to avoid collision with the hypothetical `collectConfigMaps` method-name style used elsewhere; the CRD JSON key stays `collectConfigMaps`.)

- [ ] **Step 2.3: Populate the new fields inside `Configure()`.**

  Inside `Configure()`, locate the `if ddaSpec.Features != nil && ddaSpec.Features.KubeStateMetricsCore != nil && apiutils.BoolValue(...Enabled)` block. Immediately after the line `f.collectCrMetrics = ddaSpec.Features.KubeStateMetricsCore.CollectCrMetrics` (feature.go:95), add:

  ```go
  	f.collectSecrets = apiutils.BoolValue(apiutils.NewBoolPointer(true))
  	if ddaSpec.Features.KubeStateMetricsCore.CollectSecretMetrics != nil {
  		f.collectSecrets = *ddaSpec.Features.KubeStateMetricsCore.CollectSecretMetrics
  	}
  	f.collectConfigMapsMetrics = true
  	if ddaSpec.Features.KubeStateMetricsCore.CollectConfigMaps != nil {
  		f.collectConfigMapsMetrics = *ddaSpec.Features.KubeStateMetricsCore.CollectConfigMaps
  	}
  	f.labelsAsTags = ddaSpec.Features.KubeStateMetricsCore.LabelsAsTags
  	f.annotationsAsTags = ddaSpec.Features.KubeStateMetricsCore.AnnotationsAsTags
  	f.tags = ddaSpec.Features.KubeStateMetricsCore.Tags
  ```

- [ ] **Step 2.4: Propagate into `collectorOptions` in `ManageDependencies`.**

  Locate the `collectorOpts := collectorOptions{...}` literal in `ManageDependencies` (feature.go:195). Extend it to:

  ```go
  	collectorOpts := collectorOptions{
  		enableVPA:                 pInfo.IsResourceSupported("VerticalPodAutoscaler"),
  		enableAPIService:          f.collectAPIServiceMetrics,
  		enableCRD:                 f.collectCRDMetrics,
  		enableControllerRevisions: f.collectControllerRevisions,
  		collectSecrets:            f.collectSecrets,
  		collectConfigMaps:         f.collectConfigMapsMetrics,
  		customResources:           f.collectCrMetrics,
  		labelsAsTags:              f.labelsAsTags,
  		annotationsAsTags:         f.annotationsAsTags,
  		tags:                      f.tags,
  	}
  ```

- [ ] **Step 2.5: Update the config-hash map so spec changes trigger re-reconcile.**

  Find the `defaultConfigData := map[string]any{...}` block inside `Configure()` (feature.go:159). Extend it to include the new options so that a change in `labelsAsTags`/`tags`/etc. produces a different hash:

  ```go
  	defaultConfigData := map[string]any{
  		"collect_crds":            f.collectCRDMetrics,
  		"collect_apiservices":     f.collectAPIServiceMetrics,
  		"collect_cr_metrics":      f.collectCrMetrics,
  		"collect_secrets":         f.collectSecrets,
  		"collect_configmaps":      f.collectConfigMapsMetrics,
  		"labels_as_tags":          f.labelsAsTags,
  		"annotations_as_tags":     f.annotationsAsTags,
  		"tags":                    f.tags,
  	}
  ```

- [ ] **Step 2.6: Run existing tests — expect failures in the configmap/rbac suites.**

  ```bash
  go test ./internal/controller/datadogagent/feature/kubernetesstatecore/... -v
  ```
  Expected: many test cases fail because `collectSecrets` / `collectConfigMaps` default to `false` in `collectorOptions` literals inside `configmap_test.go` and `rbac_test.go` but the runtime now defaults them to `true` for enabled KSM. These failures will be resolved in Tasks 6 and 7 where those call sites are updated. Keep going.

  (If you want a quick green tree after step 2.6, flip the test baselines now — but it is cleaner to wire per-field and fix tests per field in Tasks 3–7. Proceed without fixing yet.)

- [ ] **Step 2.7: Revert the test-suite failures by temporarily making `collectSecrets` / `collectConfigMaps` `false` when the feature struct is zero-valued.**

  Do nothing here — the `collectorOptions` zero-value defaults are intentional; the real end state is that `feature_test.go` populates `collectSecrets: true, collectConfigMaps: true` whenever KSM is enabled, which Task 6/7 will assert. Skip this step.

- [ ] **Step 2.8: Commit the wiring change (tests expected to be temporarily red).**

  Only commit if the compile succeeds. Do not commit if `go build ./...` fails.

  ```bash
  go build ./...
  git add internal/controller/datadogagent/feature/kubernetesstatecore/feature.go
  git commit -m "[CONTP-1569] Plumb new KSM options through Configure and collectorOptions

  Wires CollectSecretMetrics / CollectConfigMaps / LabelsAsTags / AnnotationsAsTags
  / Tags from the DDA spec into the ksmFeature struct and collectorOptions. No YAML
  or RBAC rendering changes yet; subsequent commits add per-field behavior + tests."
  ```

  > Note: tests in this package are red until Task 7 completes. This is the only intentional red commit in the sequence — Tasks 3–7 each end with a green test run.

---

## Task 3 — `tags` (static tags)

**Files:**
- Modify: `internal/controller/datadogagent/feature/kubernetesstatecore/configmap.go:50-117` (function `ksmCheckConfig`)
- Modify: `internal/controller/datadogagent/feature/kubernetesstatecore/configmap_yaml_test.go`
- Modify: `internal/controller/datadogagent/feature/kubernetesstatecore/configmap_test.go`
- Modify: `pkg/testutils/builder.go`

- [ ] **Step 3.1: Write the failing YAML unit test.**

  In `configmap_yaml_test.go`, after the existing `"custom resources with proper indentation"` case inside `TestKsmCheckConfigYAMLFormat`'s `testCases` slice, append:

  ```go
  	{
  		name:         "tags are emitted as a YAML list",
  		clusterCheck: false,
  		collectorOpts: collectorOptions{
  			tags: []string{"env:prod", "team:cont-p"},
  		},
  		validateFunc: func(t *testing.T, output string) {
  			var config map[string]any
  			require.NoError(t, yaml.Unmarshal([]byte(output), &config), "YAML should be valid")
  			instances := config["instances"].([]any)
  			require.Len(t, instances, 1)
  			instance := instances[0].(map[string]any)
  			tags, ok := instance["tags"].([]any)
  			require.True(t, ok, "tags should exist as a list")
  			assert.Equal(t, []any{"env:prod", "team:cont-p"}, tags)
  		},
  	},
  	{
  		name:         "tags omitted when empty",
  		clusterCheck: false,
  		collectorOpts: collectorOptions{},
  		validateFunc: func(t *testing.T, output string) {
  			assert.NotContains(t, output, "tags:", "tags key should be absent when not configured")
  		},
  	},
  ```

- [ ] **Step 3.2: Run the test to confirm it fails.**

  ```bash
  go test ./internal/controller/datadogagent/feature/kubernetesstatecore/ -run TestKsmCheckConfigYAMLFormat -v
  ```
  Expected: new subtest `tags are emitted as a YAML list` FAILS ("tags key not present" or similar).

- [ ] **Step 3.3: Implement `tags` emission in `configmap.go`.**

  Open `configmap.go`. Locate the end of `ksmCheckConfig()` — just before `return config.String()`. Insert (after the `customResources` block, before `return`):

  ```go
  	if len(collectorOpts.tags) > 0 {
  		config.WriteString("    tags:\n")
  		for _, t := range collectorOpts.tags {
  			fmt.Fprintf(config, "    - %s\n", t)
  		}
  	}
  ```

  Also add `"fmt"` to the `import` block at the top of the file if not present.

- [ ] **Step 3.4: Run the test to confirm it passes.**

  ```bash
  go test ./internal/controller/datadogagent/feature/kubernetesstatecore/ -run TestKsmCheckConfigYAMLFormat -v
  ```
  Expected: all subtests PASS.

- [ ] **Step 3.5: Add a `WithKSMTags` builder helper.**

  In `pkg/testutils/builder.go`, after the existing `WithKSMCustomConf` function (line 666), append:

  ```go
  func (builder *DatadogAgentBuilder) WithKSMTags(tags []string) *DatadogAgentBuilder {
  	builder.initKSM()
  	builder.datadogAgent.Spec.Features.KubeStateMetricsCore.Tags = tags
  	return builder
  }
  ```

- [ ] **Step 3.6: Extend `configmap_test.go` coverage.**

  In `configmap_test.go`, after `optionsWithVPAAndCustomResources` declaration (around line 69), add:

  ```go
  	optionsWithTags := collectorOptions{tags: []string{"env:prod"}}
  ```

  In the `tests` slice, append:

  ```go
  	{
  		name: "with tags",
  		fields: fields{
  			owner:                    owner,
  			enable:                   true,
  			runInClusterChecksRunner: true,
  			configConfigMapName:      defaultKubeStateMetricsCoreConf,
  			collectorOpts:            optionsWithTags,
  		},
  		want: buildDefaultConfigMap(owner.GetNamespace(), defaultKubeStateMetricsCoreConf, ksmCheckConfig(true, optionsWithTags)),
  	},
  ```

- [ ] **Step 3.7: Run the full KSM package test suite.**

  ```bash
  go test ./internal/controller/datadogagent/feature/kubernetesstatecore/... -v
  ```
  Expected: the three YAML cases for `tags` PASS; remaining unrelated failures from Task 2 remain (Tasks 4–7 resolve them).

- [ ] **Step 3.8: Commit.**

  ```bash
  git add internal/controller/datadogagent/feature/kubernetesstatecore/configmap.go \
          internal/controller/datadogagent/feature/kubernetesstatecore/configmap_test.go \
          internal/controller/datadogagent/feature/kubernetesstatecore/configmap_yaml_test.go \
          pkg/testutils/builder.go
  git commit -m "[CONTP-1569] Emit KSM static tags in check instance YAML"
  ```

---

## Task 4 — `labelsAsTags`

**Files:**
- Modify: `internal/controller/datadogagent/feature/kubernetesstatecore/configmap.go` (`ksmCheckConfig`)
- Modify: `internal/controller/datadogagent/feature/kubernetesstatecore/configmap_yaml_test.go`
- Modify: `pkg/testutils/builder.go`

- [ ] **Step 4.1: Write the failing YAML unit test.**

  Append to the `testCases` slice inside `TestKsmCheckConfigYAMLFormat`:

  ```go
  	{
  		name:         "labels_as_tags emits nested map",
  		clusterCheck: false,
  		collectorOpts: collectorOptions{
  			labelsAsTags: map[string]map[string]string{
  				"pod":  {"app": "app"},
  				"node": {"zone": "zone", "team": "team"},
  			},
  		},
  		validateFunc: func(t *testing.T, output string) {
  			var config map[string]any
  			require.NoError(t, yaml.Unmarshal([]byte(output), &config))
  			instance := config["instances"].([]any)[0].(map[string]any)
  			lat, ok := instance["labels_as_tags"].(map[string]any)
  			require.True(t, ok, "labels_as_tags map should exist")
  			pod := lat["pod"].(map[string]any)
  			assert.Equal(t, "app", pod["app"])
  			node := lat["node"].(map[string]any)
  			assert.Equal(t, "zone", node["zone"])
  			assert.Equal(t, "team", node["team"])
  		},
  	},
  	{
  		name:          "labels_as_tags omitted when empty",
  		clusterCheck:  false,
  		collectorOpts: collectorOptions{},
  		validateFunc: func(t *testing.T, output string) {
  			assert.NotContains(t, output, "labels_as_tags")
  		},
  	},
  ```

- [ ] **Step 4.2: Confirm failure.**

  ```bash
  go test ./internal/controller/datadogagent/feature/kubernetesstatecore/ -run TestKsmCheckConfigYAMLFormat -v
  ```
  Expected: the new `labels_as_tags emits nested map` subtest FAILS.

- [ ] **Step 4.3: Implement `labels_as_tags` emission.**

  In `configmap.go`, add this block just before the `if len(collectorOpts.tags) > 0 { ... }` block added in Task 3:

  ```go
  	if len(collectorOpts.labelsAsTags) > 0 {
  		config.WriteString("    labels_as_tags:\n")
  		indentedWriter := newIndentWriter(config, 6)
  		encoder := yaml.NewEncoder(indentedWriter)
  		encoder.SetIndent(2)
  		if err := encoder.Encode(collectorOpts.labelsAsTags); err != nil {
  			return config.String()
  		}
  		encoder.Close()
  	}
  ```

  (`yaml` and `newIndentWriter` are already imported in `configmap.go`.)

- [ ] **Step 4.4: Confirm pass.**

  ```bash
  go test ./internal/controller/datadogagent/feature/kubernetesstatecore/ -run TestKsmCheckConfigYAMLFormat -v
  ```
  Expected: all subtests PASS.

- [ ] **Step 4.5: Add builder helper.**

  Append to `pkg/testutils/builder.go`:

  ```go
  func (builder *DatadogAgentBuilder) WithKSMLabelsAsTags(m map[string]map[string]string) *DatadogAgentBuilder {
  	builder.initKSM()
  	builder.datadogAgent.Spec.Features.KubeStateMetricsCore.LabelsAsTags = m
  	return builder
  }
  ```

- [ ] **Step 4.6: Extend `configmap_test.go`.**

  Add to the options declarations:

  ```go
  	optionsWithLabelsAsTags := collectorOptions{
  		labelsAsTags: map[string]map[string]string{"pod": {"app": "app"}},
  	}
  ```

  Add to the `tests` slice:

  ```go
  	{
  		name: "with labelsAsTags",
  		fields: fields{
  			owner:                    owner,
  			enable:                   true,
  			runInClusterChecksRunner: true,
  			configConfigMapName:      defaultKubeStateMetricsCoreConf,
  			collectorOpts:            optionsWithLabelsAsTags,
  		},
  		want: buildDefaultConfigMap(owner.GetNamespace(), defaultKubeStateMetricsCoreConf, ksmCheckConfig(true, optionsWithLabelsAsTags)),
  	},
  ```

- [ ] **Step 4.7: Run the KSM package suite.**

  ```bash
  go test ./internal/controller/datadogagent/feature/kubernetesstatecore/... -v
  ```
  Expected: `labels_as_tags` cases PASS; task-6/7-affected cases still may be red.

- [ ] **Step 4.8: Commit.**

  ```bash
  git add internal/controller/datadogagent/feature/kubernetesstatecore/configmap.go \
          internal/controller/datadogagent/feature/kubernetesstatecore/configmap_test.go \
          internal/controller/datadogagent/feature/kubernetesstatecore/configmap_yaml_test.go \
          pkg/testutils/builder.go
  git commit -m "[CONTP-1569] Emit KSM labels_as_tags map in check instance YAML"
  ```

---

## Task 5 — `annotationsAsTags`

Mirror image of Task 4 — same file, same shape (`map[string]map[string]string`), YAML key `annotations_as_tags`.

**Files:**
- Modify: `internal/controller/datadogagent/feature/kubernetesstatecore/configmap.go`
- Modify: `internal/controller/datadogagent/feature/kubernetesstatecore/configmap_yaml_test.go`
- Modify: `internal/controller/datadogagent/feature/kubernetesstatecore/configmap_test.go`
- Modify: `pkg/testutils/builder.go`

- [ ] **Step 5.1: Write the failing test.**

  Append to `TestKsmCheckConfigYAMLFormat.testCases`:

  ```go
  	{
  		name:         "annotations_as_tags emits nested map",
  		clusterCheck: false,
  		collectorOpts: collectorOptions{
  			annotationsAsTags: map[string]map[string]string{
  				"pod": {"tags_datadoghq_com_version": "version"},
  			},
  		},
  		validateFunc: func(t *testing.T, output string) {
  			var config map[string]any
  			require.NoError(t, yaml.Unmarshal([]byte(output), &config))
  			instance := config["instances"].([]any)[0].(map[string]any)
  			aat, ok := instance["annotations_as_tags"].(map[string]any)
  			require.True(t, ok)
  			pod := aat["pod"].(map[string]any)
  			assert.Equal(t, "version", pod["tags_datadoghq_com_version"])
  		},
  	},
  	{
  		name:          "annotations_as_tags omitted when empty",
  		clusterCheck:  false,
  		collectorOpts: collectorOptions{},
  		validateFunc: func(t *testing.T, output string) {
  			assert.NotContains(t, output, "annotations_as_tags")
  		},
  	},
  ```

- [ ] **Step 5.2: Confirm failure.**

  ```bash
  go test ./internal/controller/datadogagent/feature/kubernetesstatecore/ -run TestKsmCheckConfigYAMLFormat -v
  ```
  Expected: `annotations_as_tags emits nested map` FAILS.

- [ ] **Step 5.3: Implement.**

  In `configmap.go`, immediately after the `labels_as_tags` block added in Task 4, add:

  ```go
  	if len(collectorOpts.annotationsAsTags) > 0 {
  		config.WriteString("    annotations_as_tags:\n")
  		indentedWriter := newIndentWriter(config, 6)
  		encoder := yaml.NewEncoder(indentedWriter)
  		encoder.SetIndent(2)
  		if err := encoder.Encode(collectorOpts.annotationsAsTags); err != nil {
  			return config.String()
  		}
  		encoder.Close()
  	}
  ```

- [ ] **Step 5.4: Confirm pass.**

  ```bash
  go test ./internal/controller/datadogagent/feature/kubernetesstatecore/ -run TestKsmCheckConfigYAMLFormat -v
  ```
  Expected: PASS.

- [ ] **Step 5.5: Add builder helper.**

  Append to `pkg/testutils/builder.go`:

  ```go
  func (builder *DatadogAgentBuilder) WithKSMAnnotationsAsTags(m map[string]map[string]string) *DatadogAgentBuilder {
  	builder.initKSM()
  	builder.datadogAgent.Spec.Features.KubeStateMetricsCore.AnnotationsAsTags = m
  	return builder
  }
  ```

- [ ] **Step 5.6: Extend `configmap_test.go`.**

  Add options:

  ```go
  	optionsWithAnnotationsAsTags := collectorOptions{
  		annotationsAsTags: map[string]map[string]string{"pod": {"tags_datadoghq_com_version": "version"}},
  	}
  ```

  Add test case (analogous to Task 4.6).

- [ ] **Step 5.7: Commit.**

  ```bash
  git add internal/controller/datadogagent/feature/kubernetesstatecore/configmap.go \
          internal/controller/datadogagent/feature/kubernetesstatecore/configmap_test.go \
          internal/controller/datadogagent/feature/kubernetesstatecore/configmap_yaml_test.go \
          pkg/testutils/builder.go
  git commit -m "[CONTP-1569] Emit KSM annotations_as_tags map in check instance YAML"
  ```

---

## Task 6 — `collectSecretMetrics`

**Files:**
- Modify: `internal/controller/datadogagent/feature/kubernetesstatecore/configmap.go` (`ksmCheckConfig`)
- Modify: `internal/controller/datadogagent/feature/kubernetesstatecore/rbac.go` (`getRBACPolicyRules`)
- Modify: `internal/controller/datadogagent/feature/kubernetesstatecore/configmap_yaml_test.go`
- Modify: `internal/controller/datadogagent/feature/kubernetesstatecore/configmap_test.go`
- Modify: `internal/controller/datadogagent/feature/kubernetesstatecore/rbac_test.go`
- Modify: `pkg/testutils/builder.go`

- [ ] **Step 6.1: Fix the existing `configmap_test.go` baselines that Task 2 rendered red.**

  Every existing `collectorOptions{...}` literal in `configmap_test.go` that represents an "enabled KSM" case must now include `collectSecrets: true, collectConfigMaps: true` so the rendered YAML matches the new runtime default. Edit each existing `optionsWith*` declaration:

  ```go
  	defaultOptions := collectorOptions{collectSecrets: true, collectConfigMaps: true}
  	optionsWithVPA := collectorOptions{enableVPA: true, collectSecrets: true, collectConfigMaps: true}
  	optionsWithCRD := collectorOptions{enableCRD: true, collectSecrets: true, collectConfigMaps: true}
  	optionsWithAPIService := collectorOptions{enableAPIService: true, collectSecrets: true, collectConfigMaps: true}
  	optionsWithControllerRevisions := collectorOptions{enableControllerRevisions: true, collectSecrets: true, collectConfigMaps: true}
  	optionsWithCustomResources := collectorOptions{collectSecrets: true, collectConfigMaps: true, customResources: ...}
  	optionsWithMultipleCustomResources := collectorOptions{collectSecrets: true, collectConfigMaps: true, customResources: ...}
  	optionsWithVPAAndCustomResources := collectorOptions{enableVPA: true, collectSecrets: true, collectConfigMaps: true, customResources: ...}
  ```

  (Preserve each struct's existing fields — just add the two new ones.)

- [ ] **Step 6.2: Fix the existing `rbac_test.go` baselines.**

  Update every existing `collectorOptions{...}` literal in `TestGetRBACPolicyRules` test cases the same way — include `collectSecrets: true, collectConfigMaps: true`.

  Update the `expectedBaseRules` core-API rule: the resource list is already `{ConfigMapsResource, EndpointsResource, ..., SecretsResource, ServicesResource}` — keep unchanged; it represents the both-true baseline.

- [ ] **Step 6.3: Write a failing test case for `collectSecrets: false`.**

  In `rbac_test.go`, append a new test case:

  ```go
  	{
  		name: "collectSecrets=false drops secrets from core rule",
  		collectorOpts: collectorOptions{collectSecrets: false, collectConfigMaps: true},
  		expectedExtraRules: []rbacv1.PolicyRule{},
  	},
  ```

  And expand the test body to also assert that when `collectSecrets` is false, `SecretsResource` is *not* present in any rule. Add after the existing assertions inside the subtest body:

  ```go
  		if !tc.collectorOpts.collectSecrets {
  			for _, rule := range rules {
  				for _, r := range rule.Resources {
  					assert.NotEqual(t, rbac.SecretsResource, r, "secrets should not appear when collectSecrets=false")
  				}
  			}
  		}
  ```

  Also add a YAML test case in `configmap_yaml_test.go`:

  ```go
  	{
  		name:         "collectSecrets=false drops secrets collector",
  		clusterCheck: false,
  		collectorOpts: collectorOptions{collectSecrets: false, collectConfigMaps: true},
  		validateFunc: func(t *testing.T, output string) {
  			var config map[string]any
  			require.NoError(t, yaml.Unmarshal([]byte(output), &config))
  			instance := config["instances"].([]any)[0].(map[string]any)
  			collectors := instance["collectors"].([]any)
  			for _, c := range collectors {
  				assert.NotEqual(t, "secrets", c, "secrets collector should be absent")
  			}
  		},
  	},
  ```

- [ ] **Step 6.4: Confirm the new tests fail and the adjusted baselines are green.**

  ```bash
  go test ./internal/controller/datadogagent/feature/kubernetesstatecore/... -v
  ```
  Expected: `collectSecrets=false drops secrets from core rule` and `collectSecrets=false drops secrets collector` FAIL. Existing baselines PASS.

- [ ] **Step 6.5: Implement in `rbac.go`.**

  Edit `getRBACPolicyRules`. The first rule currently hardcodes `ConfigMapsResource` and `SecretsResource` in the resources list. Replace the first rule block with:

  ```go
  	coreResources := []string{
  		rbac.ConfigMapsResource,
  		rbac.EndpointsResource,
  		rbac.EventsResource,
  		rbac.LimitRangesResource,
  		rbac.NamespaceResource,
  		rbac.NodesResource,
  		rbac.PersistentVolumeClaimsResource,
  		rbac.PersistentVolumesResource,
  		rbac.PodsResource,
  		rbac.ReplicationControllersResource,
  		rbac.ResourceQuotasResource,
  		rbac.SecretsResource,
  		rbac.ServicesResource,
  	}
  	if !collectorOpts.collectSecrets {
  		coreResources = slices.DeleteFunc(coreResources, func(s string) bool { return s == rbac.SecretsResource })
  	}
  	if !collectorOpts.collectConfigMaps {
  		coreResources = slices.DeleteFunc(coreResources, func(s string) bool { return s == rbac.ConfigMapsResource })
  	}

  	rbacRules := []rbacv1.PolicyRule{
  		{
  			APIGroups: []string{rbac.CoreAPIGroup},
  			Resources: coreResources,
  		},
  ```

  (Add `"slices"` to the imports at the top of `rbac.go`.)

- [ ] **Step 6.6: Implement in `configmap.go`.**

  In `ksmCheckConfig`, the current collector list hardcodes `- configmaps` and `- secrets`. Remove those two lines from the hardcoded list and, *before* the `- pods` line in the list, add:

  ```go
  	if collectorOpts.collectSecrets {
  		config.WriteString("    - secrets\n")
  	}
  	if collectorOpts.collectConfigMaps {
  		config.WriteString("    - configmaps\n")
  	}
  ```

  Replace the existing literal string block that lists collectors with a version that omits `- configmaps` and `- secrets` from the fixed list. Keep all other collector entries unchanged.

- [ ] **Step 6.7: Confirm tests pass.**

  ```bash
  go test ./internal/controller/datadogagent/feature/kubernetesstatecore/... -v
  ```
  Expected: all PASS.

- [ ] **Step 6.8: Add builder helper.**

  Append to `pkg/testutils/builder.go`:

  ```go
  func (builder *DatadogAgentBuilder) WithKSMCollectSecretMetrics(enabled bool) *DatadogAgentBuilder {
  	builder.initKSM()
  	builder.datadogAgent.Spec.Features.KubeStateMetricsCore.CollectSecretMetrics = ptr.To(enabled)
  	return builder
  }
  ```

- [ ] **Step 6.9: Commit.**

  ```bash
  git add internal/controller/datadogagent/feature/kubernetesstatecore/configmap.go \
          internal/controller/datadogagent/feature/kubernetesstatecore/rbac.go \
          internal/controller/datadogagent/feature/kubernetesstatecore/configmap_test.go \
          internal/controller/datadogagent/feature/kubernetesstatecore/configmap_yaml_test.go \
          internal/controller/datadogagent/feature/kubernetesstatecore/rbac_test.go \
          pkg/testutils/builder.go
  git commit -m "[CONTP-1569] Honor CollectSecretMetrics in KSM config + RBAC"
  ```

---

## Task 7 — `collectConfigMaps`

Mirror of Task 6 but for ConfigMaps. Because Task 6 already updated the baselines to `collectConfigMaps: true` and moved configmaps-list emission to a conditional, this task is small.

- [ ] **Step 7.1: Add failing tests.**

  In `rbac_test.go`:

  ```go
  	{
  		name: "collectConfigMaps=false drops configmaps from core rule",
  		collectorOpts: collectorOptions{collectSecrets: true, collectConfigMaps: false},
  		expectedExtraRules: []rbacv1.PolicyRule{},
  	},
  ```

  Extend the subtest body assertion (alongside Task 6.3's) with a symmetric check:

  ```go
  		if !tc.collectorOpts.collectConfigMaps {
  			for _, rule := range rules {
  				for _, r := range rule.Resources {
  					assert.NotEqual(t, rbac.ConfigMapsResource, r, "configmaps should not appear when collectConfigMaps=false")
  				}
  			}
  		}
  ```

  In `configmap_yaml_test.go`:

  ```go
  	{
  		name:         "collectConfigMaps=false drops configmaps collector",
  		clusterCheck: false,
  		collectorOpts: collectorOptions{collectSecrets: true, collectConfigMaps: false},
  		validateFunc: func(t *testing.T, output string) {
  			var config map[string]any
  			require.NoError(t, yaml.Unmarshal([]byte(output), &config))
  			instance := config["instances"].([]any)[0].(map[string]any)
  			collectors := instance["collectors"].([]any)
  			for _, c := range collectors {
  				assert.NotEqual(t, "configmaps", c)
  			}
  		},
  	},
  ```

- [ ] **Step 7.2: Confirm failures then pass.**

  Because Task 6 already made the implementation conditional on both flags, these new tests should **pass immediately** — which means this task is test-only. Run:

  ```bash
  go test ./internal/controller/datadogagent/feature/kubernetesstatecore/... -v
  ```
  Expected: all PASS. (If they don't, Task 6.5–6.6 missed the `collectConfigMaps` conditional — fix there.)

- [ ] **Step 7.3: Add builder helper.**

  Append to `pkg/testutils/builder.go`:

  ```go
  func (builder *DatadogAgentBuilder) WithKSMCollectConfigMaps(enabled bool) *DatadogAgentBuilder {
  	builder.initKSM()
  	builder.datadogAgent.Spec.Features.KubeStateMetricsCore.CollectConfigMaps = ptr.To(enabled)
  	return builder
  }
  ```

- [ ] **Step 7.4: Commit.**

  ```bash
  git add internal/controller/datadogagent/feature/kubernetesstatecore/configmap_yaml_test.go \
          internal/controller/datadogagent/feature/kubernetesstatecore/rbac_test.go \
          pkg/testutils/builder.go
  git commit -m "[CONTP-1569] Test CollectConfigMaps toggle in KSM config + RBAC"
  ```

---

## Task 8 — `Test_ksmFeature_Configure` end-to-end cases

Exercise the plumbing from the DDA builder all the way through `Configure()` to confirm the feature struct carries the new fields.

**Files:**
- Modify: `internal/controller/datadogagent/feature/kubernetesstatecore/feature_test.go`

- [ ] **Step 8.1: Add a Configure test case for each new field.**

  Inside `Test_ksmFeature_Configure`'s `FeatureTestSuite` literal, append:

  ```go
  	{
  		Name: "ksm-core with CollectSecretMetrics=false",
  		DDA: testutils.NewDatadogAgentBuilder().
  			WithKSMEnabled(true).
  			WithKSMCollectSecretMetrics(false).
  			Build(),
  		WantConfigure: true,
  		ClusterAgent:  ksmClusterAgentWantFunc(false),
  		Agent:         test.NewDefaultComponentTest().WithWantFunc(ksmAgentNodeWantFunc),
  	},
  	{
  		Name: "ksm-core with CollectConfigMaps=false",
  		DDA: testutils.NewDatadogAgentBuilder().
  			WithKSMEnabled(true).
  			WithKSMCollectConfigMaps(false).
  			Build(),
  		WantConfigure: true,
  		ClusterAgent:  ksmClusterAgentWantFunc(false),
  		Agent:         test.NewDefaultComponentTest().WithWantFunc(ksmAgentNodeWantFunc),
  	},
  	{
  		Name: "ksm-core with labelsAsTags",
  		DDA: testutils.NewDatadogAgentBuilder().
  			WithKSMEnabled(true).
  			WithKSMLabelsAsTags(map[string]map[string]string{"pod": {"app": "app"}}).
  			Build(),
  		WantConfigure: true,
  		ClusterAgent:  ksmClusterAgentWantFunc(false),
  		Agent:         test.NewDefaultComponentTest().WithWantFunc(ksmAgentNodeWantFunc),
  	},
  	{
  		Name: "ksm-core with annotationsAsTags",
  		DDA: testutils.NewDatadogAgentBuilder().
  			WithKSMEnabled(true).
  			WithKSMAnnotationsAsTags(map[string]map[string]string{"pod": {"version": "version"}}).
  			Build(),
  		WantConfigure: true,
  		ClusterAgent:  ksmClusterAgentWantFunc(false),
  		Agent:         test.NewDefaultComponentTest().WithWantFunc(ksmAgentNodeWantFunc),
  	},
  	{
  		Name: "ksm-core with tags",
  		DDA: testutils.NewDatadogAgentBuilder().
  			WithKSMEnabled(true).
  			WithKSMTags([]string{"env:prod"}).
  			Build(),
  		WantConfigure: true,
  		ClusterAgent:  ksmClusterAgentWantFunc(false),
  		Agent:         test.NewDefaultComponentTest().WithWantFunc(ksmAgentNodeWantFunc),
  	},
  ```

- [ ] **Step 8.2: Run the full package suite.**

  ```bash
  go test ./internal/controller/datadogagent/feature/kubernetesstatecore/... -v
  ```
  Expected: all PASS.

- [ ] **Step 8.3: Run the broader test sweep.**

  ```bash
  go test ./...
  ```
  Expected: all PASS. (Run from repo root; this is fast — not `make test`.)

- [ ] **Step 8.4: Run linter.**

  ```bash
  make lint
  ```
  Expected: no new issues.

- [ ] **Step 8.5: Commit.**

  ```bash
  git add internal/controller/datadogagent/feature/kubernetesstatecore/feature_test.go
  git commit -m "[CONTP-1569] Add Configure() end-to-end tests for new KSM fields"
  ```

---

## Task 9 — Cloud-workspace smoke test

The goal here is to validate that a real KSM-enabled cluster picks up the new fields and produces the expected ConfigMap + ClusterRole.

Reference: `../datadog-agent/workspace.md` for connection and build commands.

- [ ] **Step 9.1: Sync branch to workspace.**

  Option A (preferred — Cursor workflow):
  ```bash
  workspaces connect minyi-agent-dev --editor cursor --repo datadog-operator
  ```
  Option B (terminal):
  ```bash
  wssh
  cd /home/bits/dd/datadog-operator
  git fetch origin
  git checkout minyi/contp-1569-ksm-bucket-a
  ```

- [ ] **Step 9.2: Build and deploy the operator image.**

  On the workspace:
  ```bash
  bash scripts/workspace_build_deploy.sh operator dev && bash scripts/workspace_build_deploy.sh deploy dev
  ```
  Expected: operator pod restarts with the new image, reaches `Running`.

- [ ] **Step 9.3: Apply a test CR exercising the new fields.**

  Edit `scripts/workspace_datadog_agent_cr.yaml` and set, under `spec.features.kubeStateMetricsCore`:

  ```yaml
  spec:
    features:
      kubeStateMetricsCore:
        enabled: true
        collectSecretMetrics: false
        collectConfigMaps: false
        labelsAsTags:
          pod:
            app: app
          node:
            zone: zone
        annotationsAsTags:
          pod:
            tags_datadoghq_com_version: version
        tags:
          - env:workspace
          - team:cont-p
  ```

  Then apply:
  ```bash
  bash scripts/workspace_build_deploy.sh deploy dev
  ```

- [ ] **Step 9.4: Verify the rendered ConfigMap.**

  ```bash
  kubectl -n datadog get configmap -l app=datadog -o name | xargs -I{} kubectl -n datadog get {} -o yaml | grep -A40 'kubernetes_state_core.yaml.default'
  ```
  Expected:
  - `collectors` list does **not** contain `- secrets` or `- configmaps`.
  - `labels_as_tags:` and `annotations_as_tags:` blocks present with the nested values.
  - `tags:` list contains `env:workspace` and `team:cont-p`.

- [ ] **Step 9.5: Verify the ClusterRole.**

  ```bash
  kubectl get clusterrole -l app.kubernetes.io/name=datadog-cluster-agent -o yaml | grep -A25 '"": '
  kubectl get clusterrole -o name | grep ksm-core
  kubectl get clusterrole <ksm-core-role> -o yaml
  ```
  Expected: the core-API rule's `resources` list does **not** include `secrets` or `configmaps`.

- [ ] **Step 9.6: Verify the cluster-agent picks up the check.**

  ```bash
  kubectl -n datadog exec deploy/datadog-cluster-agent -- agent status | sed -n '/kubernetes_state_core/,/^$/p' | head -40
  ```
  Expected: check reports `[OK]` and metric emission includes the new `tags`.

- [ ] **Step 9.7: Record outcome.**

  Paste the final `agent status` snippet and the filtered ConfigMap YAML into the CONTP-1569 ticket as the validation evidence.

---

## Task 10 — PR

- [ ] **Step 10.1: Push branch and open PR.**

  ```bash
  git push -u origin minyi/contp-1569-ksm-bucket-a
  gh pr create --base main --title "[CONTP-1569] KSM Core advanced options — Bucket A (5 fields)" --body "$(cat <<'EOF'
  ## Summary
  - Adds five Helm-parity fields to `KubeStateMetricsCoreFeatureConfig`: `collectSecretMetrics`, `collectConfigMaps`, `labelsAsTags`, `annotationsAsTags`, `tags`.
  - Fields default to prior operator behavior (collectors on; maps/tags empty).
  - Threads each field through `collectorOptions` into the rendered check-instance YAML and, for the two toggles, the ClusterRole.

  ## Scope notes
  - Intentionally excludes `namespaces` (separate ticket — RBAC topology change) and `collectVpaMetrics` / `collectCrdMetrics` / `collectApiServicesMetrics` (need precedence-policy decision vs. existing version/platform gates).

  ## Test plan
  - [x] Unit tests extended in `configmap_test.go`, `configmap_yaml_test.go`, `rbac_test.go`, `feature_test.go`.
  - [x] `go test ./...` green.
  - [x] `make lint` green.
  - [x] Cloud-workspace smoke test: ConfigMap + ClusterRole verified with `collectSecretMetrics=false`, `collectConfigMaps=false`, populated `labelsAsTags`/`annotationsAsTags`/`tags`.
  EOF
  )"
  ```

---

## Self-Review (post-draft)

- **Spec coverage:** Each of the 5 in-scope fields has (a) CRD schema in Task 1, (b) feature-level plumbing in Task 2, (c) YAML rendering in Tasks 3/4/5/6, (d) RBAC rendering in Task 6 (both toggles), (e) unit tests per field in `configmap_yaml_test.go`, `configmap_test.go`, `rbac_test.go`, (f) end-to-end Configure tests in Task 8, (g) live cluster smoke test in Task 9.
- **Deferred items** (`namespaces`, `collectVpaMetrics`, `collectCrdMetrics`, `collectApiServicesMetrics`) are documented in the Scope section as explicitly out of scope — no follow-up task here; they get their own tickets.
- **Placeholder scan:** no "TBD", "fill in", "appropriate error handling" language. All code blocks have full literals.
- **Type consistency:** CRD field names (`CollectSecretMetrics`, `CollectConfigMaps`, `LabelsAsTags`, `AnnotationsAsTags`, `Tags`), feature-struct fields (`collectSecrets`, `collectConfigMapsMetrics`, `labelsAsTags`, `annotationsAsTags`, `tags`), and `collectorOptions` fields (`collectSecrets`, `collectConfigMaps`, `labelsAsTags`, `annotationsAsTags`, `tags`) are used consistently across tasks. Builder helpers named `WithKSMCollectSecretMetrics`, `WithKSMCollectConfigMaps`, `WithKSMLabelsAsTags`, `WithKSMAnnotationsAsTags`, `WithKSMTags` are consistent between Task 3–7 definition and Task 8 use.
- **Intentional red commit:** Task 2.8 produces the only test-red commit in the sequence (wiring before Tasks 3–7 fix baselines). Called out explicitly so reviewers don't bisect to a false breakage.
