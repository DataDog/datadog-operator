// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package migrate

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/yaml"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/comparison"
)

const (
	testNamespace = "default"
	testName      = "checkout-latency"
	testMonitorID = 123456
)

func syncedMonitor(t *testing.T) *v1alpha1.DatadogMonitor {
	t.Helper()
	dm := &v1alpha1.DatadogMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:  testNamespace,
			Name:       testName,
			Finalizers: []string{"finalizer.monitor.datadoghq.com"},
			Labels:     map[string]string{"team": "checkout", "kustomize.toolkit.fluxcd.io/name": "apps"},
			Annotations: map[string]string{
				"owner":                                "checkout-team",
				"argocd.argoproj.io/tracking-id":       "apps:datadoghq.com/DatadogMonitor:default/checkout-latency",
				corev1.LastAppliedConfigAnnotation:     "{}",
				"kubectl.kubernetes.io/restartedAt":    "now",
				v1alpha1.ManagementPolicyAnnotationKey: "",
			},
		},
		Spec: v1alpha1.DatadogMonitorSpec{
			Name:    "Checkout latency",
			Message: "Checkout latency is high @slack-checkout",
			Query:   "avg(last_5m):avg:trace.http.request.duration{service:checkout} > 0.5",
			Type:    v1alpha1.DatadogMonitorTypeMetric,
			Tags:    []string{"team:checkout", "generated:kubernetes"},
			Options: v1alpha1.DatadogMonitorOptions{
				Thresholds: &v1alpha1.DatadogMonitorOptionsThresholds{Critical: new("0.5")},
			},
		},
	}
	delete(dm.Annotations, v1alpha1.ManagementPolicyAnnotationKey)
	hash, err := comparison.GenerateMD5ForSpec(&dm.Spec)
	require.NoError(t, err)
	dm.Status.ID = testMonitorID
	dm.Status.CurrentHash = hash
	dm.Status.Primary = true
	return dm
}

func newTestMigrator(t *testing.T, objs ...client.Object) (*migrator, client.Client, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	s := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(s))
	require.NoError(t, v1alpha1.AddToScheme(s))
	c := fake.NewClientBuilder().WithScheme(s).
		WithStatusSubresource(&v1alpha1.DatadogMonitor{}, &v1alpha1.DatadogGenericResource{}).
		WithObjects(objs...).
		Build()
	out, errOut := &bytes.Buffer{}, &bytes.Buffer{}
	m := newMigrator(c, out, errOut, testNamespace, testName, 2*time.Second)
	m.pollInterval = 10 * time.Millisecond
	return m, c, out, errOut
}

// fakeOperator plays the DatadogMonitor and DatadogGenericResource
// controllers: it acknowledges the orphan annotation, releases the finalizer of
// an orphaned DatadogMonitor, and adopts once the DatadogMonitor is gone.
func fakeOperator(t *testing.T, c client.Client, monitorDeletes *atomic.Int32) (stop func()) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		for ctx.Err() == nil {
			time.Sleep(5 * time.Millisecond)
			key := client.ObjectKey{Namespace: testNamespace, Name: testName}
			dm := &v1alpha1.DatadogMonitor{}
			dmErr := c.Get(ctx, key, dm)
			if dmErr == nil {
				if v1alpha1.IsOrphaned(dm) && !monitorOrphaned(dm) {
					dm.Status.Conditions = append(dm.Status.Conditions, v1alpha1.DatadogMonitorCondition{Type: v1alpha1.DatadogMonitorConditionTypeOrphaned, Status: corev1.ConditionTrue})
					_ = c.Status().Update(ctx, dm)
					continue
				}
				if !dm.DeletionTimestamp.IsZero() {
					if !v1alpha1.IsOrphaned(dm) {
						monitorDeletes.Add(1)
					}
					dm.Finalizers = nil
					_ = c.Update(ctx, dm)
				}
				continue
			}
			ddgr := &v1alpha1.DatadogGenericResource{}
			if apierrors.IsNotFound(dmErr) && c.Get(ctx, key, ddgr) == nil && ddgr.Status.Id == "" {
				ddgr.Status.Id = ddgr.Spec.ID
				now := metav1.Now()
				for _, ct := range []string{conditionAdopted, conditionUpdated} {
					meta.SetStatusCondition(&ddgr.Status.Conditions, metav1.Condition{Type: ct, Status: metav1.ConditionTrue, Reason: ct, LastTransitionTime: now})
				}
				_ = c.Status().Update(ctx, ddgr)
			}
		}
	}()
	return func() { cancel(); <-done }
}

func TestBuildDDGR(t *testing.T) {
	dm := syncedMonitor(t)
	ddgr, tracked, err := buildDDGR(dm)
	require.NoError(t, err)

	assert.True(t, tracked, "GitOps tracking metadata is detected")
	assert.Equal(t, testNamespace, ddgr.Namespace)
	assert.Equal(t, testName, ddgr.Name)
	assert.Equal(t, v1alpha1.Monitor, ddgr.Spec.Type)
	assert.Equal(t, "123456", ddgr.Spec.ID)
	assert.Equal(t, map[string]string{"team": "checkout"}, ddgr.Labels)
	assert.Equal(t, map[string]string{"owner": "checkout-team", "kubectl.kubernetes.io/restartedAt": "now"}, ddgr.Annotations)

	var payload map[string]any
	require.NoError(t, json.Unmarshal([]byte(ddgr.Spec.JsonSpec), &payload))
	assert.Equal(t, "Checkout latency", payload["name"])
	assert.Equal(t, "metric alert", payload["type"])
	assert.Equal(t, dm.Spec.Query, payload["query"])
	assert.ElementsMatch(t, []any{"generated:kubernetes", "team:checkout"}, payload["tags"], "the injected tag is kept")
	assert.Equal(t, 0.5, payload["options"].(map[string]any)["thresholds"].(map[string]any)["critical"])
	assert.NotContains(t, payload, "id", "response-only fields are not in jsonSpec")
	assert.NotContains(t, payload, "restricted_roles")
}

func TestBuildDDGR_DoesNotModifyTheMonitor(t *testing.T) {
	dm := syncedMonitor(t)
	_, _, err := buildDDGR(dm)
	require.NoError(t, err)
	assert.Equal(t, syncedMonitor(t).Spec, dm.Spec)
	assert.NoError(t, preflight(dm), "still in sync")
}

func TestBuildDDGR_NeverOrphaned(t *testing.T) {
	dm := syncedMonitor(t)
	dm.Annotations[v1alpha1.ManagementPolicyAnnotationKey] = v1alpha1.ManagementPolicyOrphan
	ddgr, _, err := buildDDGR(dm)
	require.NoError(t, err)
	assert.False(t, v1alpha1.IsOrphaned(ddgr), "the orphan annotation of the old CR is not copied")
}

func TestPreflight(t *testing.T) {
	now := metav1.Now()
	for _, tt := range []struct {
		name    string
		edit    func(*v1alpha1.DatadogMonitor)
		wantErr string
	}{
		{name: "in sync", edit: func(*v1alpha1.DatadogMonitor) {}},
		{name: "no monitor ID", edit: func(dm *v1alpha1.DatadogMonitor) { dm.Status.ID = 0 }, wantErr: "no monitor ID"},
		{name: "spec changed", edit: func(dm *v1alpha1.DatadogMonitor) { dm.Spec.Message = "changed" }, wantErr: "not in sync"},
		{name: "error condition", edit: func(dm *v1alpha1.DatadogMonitor) {
			dm.Status.Conditions = []v1alpha1.DatadogMonitorCondition{{Type: v1alpha1.DatadogMonitorConditionTypeError, Status: corev1.ConditionTrue, Message: "boom"}}
		}, wantErr: "has an error"},
		{name: "resolved error condition", edit: func(dm *v1alpha1.DatadogMonitor) {
			dm.Status.Conditions = []v1alpha1.DatadogMonitorCondition{{Type: v1alpha1.DatadogMonitorConditionTypeError, Status: corev1.ConditionFalse}}
		}},
		{name: "being deleted without the annotation", edit: func(dm *v1alpha1.DatadogMonitor) { dm.DeletionTimestamp = &now }, wantErr: "being deleted"},
		{name: "being deleted with the annotation", edit: func(dm *v1alpha1.DatadogMonitor) {
			dm.DeletionTimestamp = &now
			dm.Annotations[v1alpha1.ManagementPolicyAnnotationKey] = v1alpha1.ManagementPolicyOrphan
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dm := syncedMonitor(t)
			tt.edit(dm)
			err := preflight(dm)
			if tt.wantErr == "" {
				assert.NoError(t, err)
			} else {
				assert.ErrorContains(t, err, tt.wantErr)
			}
		})
	}
}

func TestMigrate(t *testing.T) {
	m, c, out, _ := newTestMigrator(t, syncedMonitor(t))
	var deletes atomic.Int32
	stop := fakeOperator(t, c, &deletes)
	defer stop()

	require.NoError(t, m.migrate(context.TODO(), false))
	assert.Contains(t, out.String(), "Migrated: DatadogGenericResource default/checkout-latency manages monitor 123456")
	assert.Equal(t, int32(0), deletes.Load(), "the monitor is never deleted")

	err := c.Get(context.TODO(), m.key, &v1alpha1.DatadogMonitor{})
	assert.True(t, apierrors.IsNotFound(err))
	ddgr := &v1alpha1.DatadogGenericResource{}
	require.NoError(t, c.Get(context.TODO(), m.key, ddgr))
	assert.Equal(t, "123456", ddgr.Spec.ID)
	assert.Equal(t, "123456", ddgr.Status.Id)
}

func TestMigrate_Resumes(t *testing.T) {
	for _, tt := range []struct {
		name  string
		setup func(t *testing.T, c client.Client, dm *v1alpha1.DatadogMonitor)
	}{
		{name: "after annotating", setup: func(t *testing.T, c client.Client, dm *v1alpha1.DatadogMonitor) {
			dm.Annotations[v1alpha1.ManagementPolicyAnnotationKey] = v1alpha1.ManagementPolicyOrphan
			require.NoError(t, c.Update(context.TODO(), dm))
		}},
		{name: "after creating the DatadogGenericResource", setup: func(t *testing.T, c client.Client, dm *v1alpha1.DatadogMonitor) {
			ddgr, _, err := buildDDGR(dm)
			require.NoError(t, err)
			require.NoError(t, c.Create(context.TODO(), ddgr))
			dm.Annotations[v1alpha1.ManagementPolicyAnnotationKey] = v1alpha1.ManagementPolicyOrphan
			require.NoError(t, c.Update(context.TODO(), dm))
		}},
		{name: "after deleting the DatadogMonitor", setup: func(t *testing.T, c client.Client, dm *v1alpha1.DatadogMonitor) {
			ddgr, _, err := buildDDGR(dm)
			require.NoError(t, err)
			require.NoError(t, c.Create(context.TODO(), ddgr))
			dm.Annotations[v1alpha1.ManagementPolicyAnnotationKey] = v1alpha1.ManagementPolicyOrphan
			require.NoError(t, c.Update(context.TODO(), dm))
			require.NoError(t, c.Delete(context.TODO(), dm))
		}},
		{name: "after the DatadogMonitor is gone", setup: func(t *testing.T, c client.Client, dm *v1alpha1.DatadogMonitor) {
			ddgr, _, err := buildDDGR(dm)
			require.NoError(t, err)
			require.NoError(t, c.Create(context.TODO(), ddgr))
			dm.Finalizers = nil
			require.NoError(t, c.Update(context.TODO(), dm))
			require.NoError(t, c.Delete(context.TODO(), dm))
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m, c, out, _ := newTestMigrator(t, syncedMonitor(t))
			dm := &v1alpha1.DatadogMonitor{}
			require.NoError(t, c.Get(context.TODO(), m.key, dm))
			tt.setup(t, c, dm)

			var deletes atomic.Int32
			stop := fakeOperator(t, c, &deletes)
			defer stop()
			require.NoError(t, m.migrate(context.TODO(), false))
			assert.Contains(t, out.String(), "Migrated:")
			assert.Equal(t, int32(0), deletes.Load())
		})
	}
}

func TestMigrate_OperatorWithoutOrphanSupport(t *testing.T) {
	// No controller acknowledges the annotation: the DatadogMonitor must not be deleted.
	m, c, _, _ := newTestMigrator(t, syncedMonitor(t))
	m.timeout = 100 * time.Millisecond
	err := m.migrate(context.TODO(), false)
	require.ErrorContains(t, err, "didn't acknowledge the orphan annotation")

	dm := &v1alpha1.DatadogMonitor{}
	require.NoError(t, c.Get(context.TODO(), m.key, dm))
	assert.True(t, dm.DeletionTimestamp.IsZero(), "the DatadogMonitor is not deleted")
	assert.True(t, apierrors.IsNotFound(c.Get(context.TODO(), m.key, &v1alpha1.DatadogGenericResource{})), "nothing else is done")
}

func TestMigrate_RefusesConflicts(t *testing.T) {
	t.Run("another DatadogGenericResource uses the ID", func(t *testing.T) {
		other := &v1alpha1.DatadogGenericResource{
			ObjectMeta: metav1.ObjectMeta{Namespace: "other", Name: "other"},
			Spec:       v1alpha1.DatadogGenericResourceSpec{Type: v1alpha1.Monitor, JsonSpec: "{}"},
			Status:     v1alpha1.DatadogGenericResourceStatus{Id: "123456"},
		}
		m, c, _, _ := newTestMigrator(t, syncedMonitor(t), other)
		require.ErrorContains(t, m.migrate(context.TODO(), false), "other/other already uses monitor 123456")
		dm := &v1alpha1.DatadogMonitor{}
		require.NoError(t, c.Get(context.TODO(), m.key, dm))
		assert.False(t, v1alpha1.IsOrphaned(dm), "nothing is changed")
	})
	t.Run("a different DatadogGenericResource has the same name", func(t *testing.T) {
		same := &v1alpha1.DatadogGenericResource{
			ObjectMeta: metav1.ObjectMeta{Namespace: testNamespace, Name: testName},
			Spec:       v1alpha1.DatadogGenericResourceSpec{Type: v1alpha1.Dashboard, JsonSpec: "{}"},
		}
		m, _, _, _ := newTestMigrator(t, syncedMonitor(t), same)
		require.ErrorContains(t, m.migrate(context.TODO(), false), "doesn't adopt monitor 123456")
	})
	t.Run("out of sync", func(t *testing.T) {
		dm := syncedMonitor(t)
		dm.Status.CurrentHash = "stale"
		m, _, _, _ := newTestMigrator(t, dm)
		require.ErrorContains(t, m.migrate(context.TODO(), false), "not in sync")
	})
}

// unchanged checks that the cluster still has only the original DatadogMonitor.
func unchanged(t *testing.T, c client.Client, m *migrator) {
	t.Helper()
	dm := &v1alpha1.DatadogMonitor{}
	require.NoError(t, c.Get(context.TODO(), m.key, dm))
	assert.False(t, v1alpha1.IsOrphaned(dm))
	assert.True(t, dm.DeletionTimestamp.IsZero())
	assert.True(t, apierrors.IsNotFound(c.Get(context.TODO(), m.key, &v1alpha1.DatadogGenericResource{})))
}

func TestMigrate_DryRun(t *testing.T) {
	m, c, out, errOut := newTestMigrator(t, syncedMonitor(t))
	require.NoError(t, m.migrate(context.TODO(), true))
	unchanged(t, c, m)
	assert.Contains(t, out.String(), "1. Annotate the DatadogMonitor")
	assert.Contains(t, out.String(), "kind: DatadogGenericResource")
	assert.Contains(t, errOut.String(), "looks managed by a GitOps tool")
}

func TestPrint_YAML(t *testing.T) {
	m, c, out, _ := newTestMigrator(t, syncedMonitor(t))
	require.NoError(t, m.print(context.TODO(), outputYAML))
	unchanged(t, c, m)

	docs := strings.Split(out.String(), "\n---\n")
	require.Len(t, docs, 2)
	dm := &v1alpha1.DatadogMonitor{}
	require.NoError(t, yaml.UnmarshalStrict([]byte(docs[0]), dm))
	assert.Equal(t, "DatadogMonitor", dm.Kind)
	assert.True(t, v1alpha1.IsOrphaned(dm))
	assert.NotContains(t, dm.Annotations, "argocd.argoproj.io/tracking-id")
	assert.Equal(t, syncedMonitor(t).Spec, dm.Spec)

	ddgr := &v1alpha1.DatadogGenericResource{}
	require.NoError(t, yaml.UnmarshalStrict([]byte(docs[1]), ddgr))
	assert.Equal(t, "DatadogGenericResource", ddgr.Kind)
	assert.Equal(t, "123456", ddgr.Spec.ID)
	assert.NotContains(t, docs[1], "\nstatus:")
	assert.NotContains(t, docs[1], "creationTimestamp")
	assert.Contains(t, docs[1], "jsonSpec: |-", "jsonSpec is a readable block")
	assert.Contains(t, docs[1], "} > 0.5", "the query is not HTML-escaped")
}

func TestPrint_JSON(t *testing.T) {
	m, c, out, _ := newTestMigrator(t, syncedMonitor(t))
	require.NoError(t, m.print(context.TODO(), outputJSON))
	unchanged(t, c, m)

	var got struct {
		ID       string         `json:"id"`
		JSONSpec map[string]any `json:"jsonSpec"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &got))
	assert.Equal(t, "123456", got.ID)
	assert.Equal(t, "Checkout latency", got.JSONSpec["name"])
}

func TestAnnotateOnly(t *testing.T) {
	m, c, out, _ := newTestMigrator(t, syncedMonitor(t))
	var deletes atomic.Int32
	stop := fakeOperator(t, c, &deletes)
	defer stop()

	require.NoError(t, m.annotateOnly(context.TODO()))
	dm := &v1alpha1.DatadogMonitor{}
	require.NoError(t, c.Get(context.TODO(), m.key, dm))
	assert.True(t, v1alpha1.IsOrphaned(dm))
	assert.True(t, dm.DeletionTimestamp.IsZero(), "not deleted")
	assert.Equal(t, "checkout-team", dm.Annotations["owner"], "other annotations are kept")
	assert.True(t, apierrors.IsNotFound(c.Get(context.TODO(), m.key, &v1alpha1.DatadogGenericResource{})))
	assert.Contains(t, out.String(), "can now be deleted or pruned")
}

func TestCheck(t *testing.T) {
	m, c, out, _ := newTestMigrator(t, syncedMonitor(t))
	require.NoError(t, m.check(context.TODO()))
	unchanged(t, c, m)
	assert.Contains(t, out.String(), "orphan annotation: no")
	assert.Contains(t, out.String(), "DatadogGenericResource default/checkout-latency: not found")
	assert.Contains(t, out.String(), "Next step: annotate the DatadogMonitor")

	var deletes atomic.Int32
	stop := fakeOperator(t, c, &deletes)
	require.NoError(t, m.migrate(context.TODO(), false))
	stop()
	out.Reset()
	require.NoError(t, m.check(context.TODO()))
	assert.Contains(t, out.String(), "DatadogMonitor default/checkout-latency: not found")
	assert.Contains(t, out.String(), "adopted: yes, updated: yes")
	assert.Contains(t, out.String(), "Next step: none, the migration is complete")
}

func TestValidate(t *testing.T) {
	o := &options{timeout: time.Minute, output: outputYAML, dryRun: true}
	assert.ErrorContains(t, o.validate(), "mutually exclusive")
	o = &options{timeout: time.Minute, output: "table"}
	assert.ErrorContains(t, o.validate(), "unsupported output")
	o = &options{timeout: time.Minute, check: true}
	assert.NoError(t, o.validate())
}
