// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package migrate

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogmonitor"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/comparison"
)

const (
	conditionAdopted = "Adopted"
	conditionUpdated = "Updated"
	conditionError   = "Error"
)

// gitOpsMetadataPrefixes are label and annotation prefixes that GitOps tools
// and Helm set to track the resources they manage. They are not copied to the
// DatadogGenericResource: a copied tracking label could make the tool prune
// it, and pruning it deletes the monitor.
var gitOpsMetadataPrefixes = []string{
	"argocd.argoproj.io/",
	"kustomize.toolkit.fluxcd.io/",
	"helm.toolkit.fluxcd.io/",
	"meta.helm.sh/",
}

// skippedMetadataKeys are other labels and annotations that are not copied.
var skippedMetadataKeys = map[string]bool{
	corev1.LastAppliedConfigAnnotation:     true,
	v1alpha1.ManagementPolicyAnnotationKey: true,
}

type migrator struct {
	client       client.Client
	out          io.Writer
	errOut       io.Writer
	key          types.NamespacedName
	timeout      time.Duration
	pollInterval time.Duration
}

func newMigrator(c client.Client, out, errOut io.Writer, namespace, name string, timeout time.Duration) *migrator {
	return &migrator{
		client:       c,
		out:          out,
		errOut:       errOut,
		key:          types.NamespacedName{Namespace: namespace, Name: name},
		timeout:      timeout,
		pollInterval: pollInterval,
	}
}

// state is what exists in the cluster. Each pointer is nil when the resource
// doesn't exist.
type state struct {
	monitor *v1alpha1.DatadogMonitor
	ddgr    *v1alpha1.DatadogGenericResource
}

func (m *migrator) load(ctx context.Context) (*state, error) {
	st := &state{}
	dm := &v1alpha1.DatadogMonitor{}
	if err := m.client.Get(ctx, m.key, dm); err == nil {
		st.monitor = dm
	} else if !apierrors.IsNotFound(err) {
		return nil, fmt.Errorf("getting DatadogMonitor %s: %w", m.key, err)
	}
	ddgr := &v1alpha1.DatadogGenericResource{}
	if err := m.client.Get(ctx, m.key, ddgr); err == nil {
		st.ddgr = ddgr
	} else if !apierrors.IsNotFound(err) {
		return nil, fmt.Errorf("getting DatadogGenericResource %s: %w", m.key, err)
	}
	return st, nil
}

func monitorID(dm *v1alpha1.DatadogMonitor) string {
	if dm.Status.ID == 0 {
		return ""
	}
	return strconv.Itoa(dm.Status.ID)
}

// preflight checks that the live monitor matches the DatadogMonitor, so that
// the DatadogGenericResource built from it doesn't change the monitor.
func preflight(dm *v1alpha1.DatadogMonitor) error {
	if !dm.DeletionTimestamp.IsZero() && !v1alpha1.IsOrphaned(dm) {
		return fmt.Errorf("DatadogMonitor %s/%s is being deleted without the %s=%s annotation, so the Operator is deleting its monitor", dm.Namespace, dm.Name, v1alpha1.ManagementPolicyAnnotationKey, v1alpha1.ManagementPolicyOrphan)
	}
	if dm.Status.ID == 0 {
		return fmt.Errorf("DatadogMonitor %s/%s has no monitor ID yet: wait for the Operator to create its monitor", dm.Namespace, dm.Name)
	}
	hash, err := comparison.GenerateMD5ForSpec(&dm.Spec)
	if err != nil {
		return fmt.Errorf("hashing the DatadogMonitor spec: %w", err)
	}
	if hash != dm.Status.CurrentHash {
		return fmt.Errorf("DatadogMonitor %s/%s is not in sync: its spec changed since the Operator last applied it, so monitor %d may not match it. Wait for the Operator to sync it, or revert the change", dm.Namespace, dm.Name, dm.Status.ID)
	}
	for _, c := range dm.Status.Conditions {
		if c.Type == v1alpha1.DatadogMonitorConditionTypeError && c.Status == corev1.ConditionTrue {
			return fmt.Errorf("DatadogMonitor %s/%s has an error, so monitor %d may not match it: %s", dm.Namespace, dm.Name, dm.Status.ID, c.Message)
		}
	}
	return nil
}

// checkDDGRs checks that no DatadogGenericResource other than the migration
// target uses the monitor ID, and that a resource with the target's name is
// the target. The Operator checks this too, but failing early changes nothing.
func (m *migrator) checkDDGRs(ctx context.Context, st *state, id string) error {
	if st.ddgr != nil && (st.ddgr.Spec.Type != v1alpha1.Monitor || st.ddgr.Spec.ID != id) {
		return fmt.Errorf("DatadogGenericResource %s already exists and doesn't adopt monitor %s (type %q, spec.id %q): rename or delete it first", m.key, id, st.ddgr.Spec.Type, st.ddgr.Spec.ID)
	}
	list := &v1alpha1.DatadogGenericResourceList{}
	if err := m.client.List(ctx, list); err != nil {
		if !apierrors.IsForbidden(err) {
			return fmt.Errorf("listing DatadogGenericResources: %w", err)
		}
		fmt.Fprintf(m.errOut, "Warning: can't list DatadogGenericResources in all namespaces; only checking namespace %s for other resources using monitor %s\n", m.key.Namespace, id)
		if err := m.client.List(ctx, list, client.InNamespace(m.key.Namespace)); err != nil {
			return fmt.Errorf("listing DatadogGenericResources: %w", err)
		}
	}
	for _, ddgr := range list.Items {
		if ddgr.Namespace == m.key.Namespace && ddgr.Name == m.key.Name {
			continue
		}
		if ddgr.Spec.Type == v1alpha1.Monitor && (ddgr.Spec.ID == id || ddgr.Status.Id == id) {
			return fmt.Errorf("DatadogGenericResource %s/%s already uses monitor %s", ddgr.Namespace, ddgr.Name, id)
		}
	}
	return nil
}

// copyMetadata copies user labels or annotations, without the ones GitOps
// tools use for tracking. It reports whether any tracking metadata was found.
func copyMetadata(in map[string]string) (map[string]string, bool) {
	tracked := false
	out := map[string]string{}
	for k, v := range in {
		if skippedMetadataKeys[k] {
			continue
		}
		if hasGitOpsPrefix(k) {
			tracked = true
			continue
		}
		out[k] = v
	}
	if len(out) == 0 {
		return nil, tracked
	}
	return out, tracked
}

func hasGitOpsPrefix(k string) bool {
	for _, prefix := range gitOpsMetadataPrefixes {
		if strings.HasPrefix(k, prefix) {
			return true
		}
	}
	return false
}

// jsonSpec is the definition the DatadogMonitor controller sends to Datadog,
// so the first update of the DatadogGenericResource doesn't change the
// monitor. It keeps the tags the controller injected, such as
// generated:kubernetes.
func jsonSpec(dm *v1alpha1.DatadogMonitor) (string, error) {
	// BuildMonitor sorts the tags of the spec in place.
	monitor, _ := datadogmonitor.BuildMonitor(logr.Discard(), dm.DeepCopy())
	data, err := json.Marshal(monitor)
	if err != nil {
		return "", fmt.Errorf("building jsonSpec: %w", err)
	}
	// Re-encode without HTML escaping, so that queries keep a readable ">".
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		return "", fmt.Errorf("building jsonSpec: %w", err)
	}
	buf := &bytes.Buffer{}
	enc := json.NewEncoder(buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(payload); err != nil {
		return "", fmt.Errorf("building jsonSpec: %w", err)
	}
	return strings.TrimSuffix(buf.String(), "\n"), nil
}

// buildDDGR builds the DatadogGenericResource that adopts the monitor of dm.
// It reports whether dm has GitOps tracking metadata.
func buildDDGR(dm *v1alpha1.DatadogMonitor) (*v1alpha1.DatadogGenericResource, bool, error) {
	spec, err := jsonSpec(dm)
	if err != nil {
		return nil, false, err
	}
	labels, trackedLabels := copyMetadata(dm.Labels)
	annotations, trackedAnnotations := copyMetadata(dm.Annotations)
	return &v1alpha1.DatadogGenericResource{
		TypeMeta: metav1.TypeMeta{APIVersion: v1alpha1.GroupVersion.String(), Kind: "DatadogGenericResource"},
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   dm.Namespace,
			Name:        dm.Name,
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: v1alpha1.DatadogGenericResourceSpec{
			Type:     v1alpha1.Monitor,
			ID:       monitorID(dm),
			JsonSpec: spec,
		},
	}, trackedLabels || trackedAnnotations, nil
}

// annotatedMonitor is the manifest of dm with the orphan annotation, for
// users who commit it before removing it from Git.
func annotatedMonitor(dm *v1alpha1.DatadogMonitor) *v1alpha1.DatadogMonitor {
	labels, _ := copyMetadata(dm.Labels)
	annotations, _ := copyMetadata(dm.Annotations)
	if annotations == nil {
		annotations = map[string]string{}
	}
	annotations[v1alpha1.ManagementPolicyAnnotationKey] = v1alpha1.ManagementPolicyOrphan
	return &v1alpha1.DatadogMonitor{
		TypeMeta: metav1.TypeMeta{APIVersion: v1alpha1.GroupVersion.String(), Kind: "DatadogMonitor"},
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   dm.Namespace,
			Name:        dm.Name,
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: *dm.Spec.DeepCopy(),
	}
}

// manifest renders obj as YAML without status and server-set metadata.
func manifest(obj runtime.Object) ([]byte, error) {
	u, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if err != nil {
		return nil, err
	}
	delete(u, "status")
	if md, ok := u["metadata"].(map[string]any); ok {
		delete(md, "creationTimestamp")
	}
	return yaml.Marshal(u)
}

// print writes manifests for GitOps users and changes nothing.
func (m *migrator) print(ctx context.Context, output string) error {
	st, err := m.load(ctx)
	if err != nil {
		return err
	}
	if st.monitor == nil {
		return fmt.Errorf("DatadogMonitor %s not found", m.key)
	}
	if err = preflight(st.monitor); err != nil {
		return err
	}
	id := monitorID(st.monitor)
	if err = m.checkDDGRs(ctx, st, id); err != nil {
		return err
	}
	ddgr, _, err := buildDDGR(st.monitor)
	if err != nil {
		return err
	}

	if output == outputJSON {
		data, marshalErr := json.MarshalIndent(struct {
			ID       string          `json:"id"`
			JSONSpec json.RawMessage `json:"jsonSpec"`
		}{ID: id, JSONSpec: json.RawMessage(ddgr.Spec.JsonSpec)}, "", "  ")
		if marshalErr != nil {
			return marshalErr
		}
		_, err = fmt.Fprintln(m.out, string(data))
		return err
	}

	dmYAML, err := manifest(annotatedMonitor(st.monitor))
	if err != nil {
		return err
	}
	ddgrYAML, err := manifest(ddgr)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(m.out, "# Step 1: commit the orphan annotation on the DatadogMonitor, and wait for it to sync.\n%s---\n# Step 2: in a later commit, replace the DatadogMonitor manifest with this one.\n%s", dmYAML, ddgrYAML)
	return err
}

// annotate adds the orphan annotation to the DatadogMonitor, then waits for
// its controller to acknowledge it with the Orphaned condition. This proves
// that the running Operator supports the annotation: an older one would
// delete the monitor along with the CR.
func (m *migrator) annotate(ctx context.Context, dm *v1alpha1.DatadogMonitor) error {
	if !v1alpha1.IsOrphaned(dm) {
		patch := client.MergeFrom(dm.DeepCopy())
		if dm.Annotations == nil {
			dm.Annotations = map[string]string{}
		}
		dm.Annotations[v1alpha1.ManagementPolicyAnnotationKey] = v1alpha1.ManagementPolicyOrphan
		if err := m.client.Patch(ctx, dm, patch); err != nil {
			return fmt.Errorf("annotating DatadogMonitor %s: %w", m.key, err)
		}
		fmt.Fprintf(m.out, "Annotated DatadogMonitor %s with %s=%s\n", m.key, v1alpha1.ManagementPolicyAnnotationKey, v1alpha1.ManagementPolicyOrphan)
	}
	if !dm.DeletionTimestamp.IsZero() {
		// Its finalizer reads the annotation when it next runs.
		return nil
	}

	fmt.Fprintf(m.out, "Waiting for the Operator to stop managing DatadogMonitor %s...\n", m.key)
	err := m.poll(ctx, func(ctx context.Context) (bool, error) {
		if err := m.client.Get(ctx, m.key, dm); err != nil {
			return false, err
		}
		return monitorOrphaned(dm), nil
	})
	if err != nil {
		return fmt.Errorf("the Operator didn't acknowledge the orphan annotation on DatadogMonitor %s (no Orphaned condition): %w. Check that the Operator runs a version that supports it and that the DatadogMonitor controller is enabled. Don't delete the DatadogMonitor until then: an Operator without orphan support deletes the monitor", m.key, err)
	}
	return nil
}

func monitorOrphaned(dm *v1alpha1.DatadogMonitor) bool {
	for _, c := range dm.Status.Conditions {
		if c.Type == v1alpha1.DatadogMonitorConditionTypeOrphaned && c.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

func (m *migrator) annotateOnly(ctx context.Context) error {
	st, err := m.load(ctx)
	if err != nil {
		return err
	}
	if st.monitor == nil {
		return fmt.Errorf("DatadogMonitor %s not found", m.key)
	}
	if err := preflight(st.monitor); err != nil {
		return err
	}
	if err := m.checkDDGRs(ctx, st, monitorID(st.monitor)); err != nil {
		return err
	}
	if err := m.annotate(ctx, st.monitor); err != nil {
		return err
	}
	fmt.Fprintf(m.out, "DatadogMonitor %s can now be deleted or pruned without deleting monitor %d. Replace its manifest with the DatadogGenericResource manifest from \"kubectl datadog monitor migrate %s -o yaml\".\n", m.key, st.monitor.Status.ID, m.key.Name)
	return nil
}

// migrate runs the migration, resuming from the current state.
//
// The DatadogGenericResource is only created once the Operator acknowledged
// the orphan annotation: from then on, the DatadogMonitor controller neither
// writes to the monitor nor deletes it, so the DatadogGenericResource can
// adopt it at any point. Creating it before deleting the DatadogMonitor means
// that either of them holds the definition at every step, so an interrupted
// run can always be resumed. The Operator usually holds the adoption until
// the DatadogMonitor is gone, but its check against legacy CRs may miss a
// DatadogMonitor created after the Operator started.
func (m *migrator) migrate(ctx context.Context, dryRun bool) error {
	st, err := m.load(ctx)
	if err != nil {
		return err
	}
	if st.monitor == nil && st.ddgr == nil {
		return fmt.Errorf("neither DatadogMonitor nor DatadogGenericResource %s found", m.key)
	}
	if st.monitor == nil {
		if st.ddgr.Spec.Type != v1alpha1.Monitor || st.ddgr.Spec.ID == "" {
			return fmt.Errorf("DatadogMonitor %s not found, and DatadogGenericResource %s doesn't adopt a monitor", m.key, m.key)
		}
		fmt.Fprintf(m.out, "DatadogMonitor %s is already gone; DatadogGenericResource %s adopts monitor %s\n", m.key, m.key, st.ddgr.Spec.ID)
		if dryRun {
			fmt.Fprintf(m.out, "Would wait for DatadogGenericResource %s to adopt monitor %s\n", m.key, st.ddgr.Spec.ID)
			return nil
		}
		return m.waitAdopted(ctx, st.ddgr.Spec.ID)
	}

	dm := st.monitor
	if err = preflight(dm); err != nil {
		return err
	}
	id := monitorID(dm)
	if err = m.checkDDGRs(ctx, st, id); err != nil {
		return err
	}
	desired, tracked, err := buildDDGR(dm)
	if err != nil {
		return err
	}
	if tracked {
		fmt.Fprintf(m.errOut, "Warning: DatadogMonitor %s looks managed by a GitOps tool or Helm. Unless you migrate it in Git (see --output yaml), the tool may recreate it, or stop tracking the DatadogGenericResource.\n", m.key)
	}

	if dryRun {
		fmt.Fprintf(m.out, "Migration plan for DatadogMonitor %s (monitor %s):\n", m.key, id)
		step := 1
		if !v1alpha1.IsOrphaned(dm) {
			fmt.Fprintf(m.out, "  %d. Annotate the DatadogMonitor with %s=%s and wait for the Operator to acknowledge it\n", step, v1alpha1.ManagementPolicyAnnotationKey, v1alpha1.ManagementPolicyOrphan)
			step++
		}
		if st.ddgr == nil {
			fmt.Fprintf(m.out, "  %d. Create the DatadogGenericResource below\n", step)
			step++
		}
		if dm.DeletionTimestamp.IsZero() {
			fmt.Fprintf(m.out, "  %d. Delete the DatadogMonitor (the monitor is kept) and wait until it's gone\n", step)
			step++
		}
		fmt.Fprintf(m.out, "  %d. Wait for the DatadogGenericResource to adopt the monitor and write its definition\n", step)
		ddgrYAML, err := manifest(desired)
		if err != nil {
			return err
		}
		fmt.Fprintf(m.out, "\n%s", ddgrYAML)
		return nil
	}

	if err := m.annotate(ctx, dm); err != nil {
		return err
	}

	if st.ddgr == nil {
		if err := m.client.Create(ctx, desired); err != nil && !apierrors.IsAlreadyExists(err) {
			return fmt.Errorf("creating DatadogGenericResource %s: %w", m.key, err)
		}
		fmt.Fprintf(m.out, "Created DatadogGenericResource %s with spec.id %s\n", m.key, id)
	}

	if dm.DeletionTimestamp.IsZero() {
		if err := m.client.Delete(ctx, dm); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("deleting DatadogMonitor %s: %w", m.key, err)
		}
		fmt.Fprintf(m.out, "Deleted DatadogMonitor %s; monitor %s is kept\n", m.key, id)
	}
	fmt.Fprintf(m.out, "Waiting for DatadogMonitor %s to be gone...\n", m.key)
	if err := m.poll(ctx, func(ctx context.Context) (bool, error) {
		err := m.client.Get(ctx, m.key, &v1alpha1.DatadogMonitor{})
		if apierrors.IsNotFound(err) {
			return true, nil
		}
		return false, err
	}); err != nil {
		return fmt.Errorf("DatadogMonitor %s is still being deleted: %w. Check that the DatadogMonitor controller is running: its finalizer releases the CR. Then run the command again", m.key, err)
	}

	return m.waitAdopted(ctx, id)
}

// waitAdopted waits for the DatadogGenericResource to adopt the monitor and
// write its definition.
func (m *migrator) waitAdopted(ctx context.Context, id string) error {
	fmt.Fprintf(m.out, "Waiting for DatadogGenericResource %s to adopt monitor %s (an adoption blocked by the DatadogMonitor is retried every requeue period, 60s by default)...\n", m.key, id)
	ddgr := &v1alpha1.DatadogGenericResource{}
	err := m.poll(ctx, func(ctx context.Context) (bool, error) {
		if err := m.client.Get(ctx, m.key, ddgr); err != nil {
			return false, err
		}
		return ddgr.Status.Id == id &&
			meta.IsStatusConditionTrue(ddgr.Status.Conditions, conditionAdopted) &&
			meta.IsStatusConditionTrue(ddgr.Status.Conditions, conditionUpdated), nil
	})
	if err != nil {
		reason := ""
		if c := meta.FindStatusCondition(ddgr.Status.Conditions, conditionError); c != nil && c.Status == metav1.ConditionTrue {
			reason = fmt.Sprintf(" (%s: %s)", c.Reason, c.Message)
		}
		return fmt.Errorf("DatadogGenericResource %s didn't adopt monitor %s%s: %w. Check that the DatadogGenericResource controller is enabled, then run the command again", m.key, id, reason, err)
	}
	fmt.Fprintf(m.out, "Migrated: DatadogGenericResource %s manages monitor %s\n", m.key, id)
	return nil
}

func (m *migrator) poll(ctx context.Context, condition wait.ConditionWithContextFunc) error {
	return wait.PollUntilContextTimeout(ctx, m.pollInterval, m.timeout, true, condition)
}

// check reports the migration state and changes nothing.
func (m *migrator) check(ctx context.Context) error {
	st, err := m.load(ctx)
	if err != nil {
		return err
	}
	var next string
	if dm := st.monitor; dm != nil {
		fmt.Fprintf(m.out, "DatadogMonitor %s: monitor %s, orphan annotation: %s, acknowledged by the Operator: %s, being deleted: %s\n",
			m.key, orDash(monitorID(dm)), yesNo(v1alpha1.IsOrphaned(dm)), yesNo(monitorOrphaned(dm)), yesNo(!dm.DeletionTimestamp.IsZero()))
		if err := preflight(dm); err != nil {
			fmt.Fprintf(m.out, "  Not ready to migrate: %v\n", err)
		}
		if !v1alpha1.IsOrphaned(dm) {
			next = "annotate the DatadogMonitor (--annotate-only, or commit the annotation) before deleting or pruning it"
		}
	} else {
		fmt.Fprintf(m.out, "DatadogMonitor %s: not found\n", m.key)
	}

	if ddgr := st.ddgr; ddgr != nil {
		fmt.Fprintf(m.out, "DatadogGenericResource %s: type %s, spec.id %s, status.id %s, adopted: %s, updated: %s\n",
			m.key, ddgr.Spec.Type, orDash(ddgr.Spec.ID), orDash(ddgr.Status.Id),
			yesNo(meta.IsStatusConditionTrue(ddgr.Status.Conditions, conditionAdopted)), yesNo(meta.IsStatusConditionTrue(ddgr.Status.Conditions, conditionUpdated)))
		if c := meta.FindStatusCondition(ddgr.Status.Conditions, conditionError); c != nil && c.Status == metav1.ConditionTrue {
			fmt.Fprintf(m.out, "  %s: %s\n", c.Reason, c.Message)
		}
	} else {
		fmt.Fprintf(m.out, "DatadogGenericResource %s: not found\n", m.key)
	}

	switch {
	case next != "":
	case st.monitor != nil:
		next = "delete the DatadogMonitor, or remove it from Git, and create the DatadogGenericResource"
	case st.ddgr == nil:
		next = "nothing to migrate"
	case meta.IsStatusConditionTrue(st.ddgr.Status.Conditions, conditionUpdated) && st.ddgr.Status.Id != "":
		next = "none, the migration is complete"
	default:
		next = "wait for the DatadogGenericResource to adopt the monitor"
	}
	fmt.Fprintf(m.out, "Next step: %s\n", next)
	return nil
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
