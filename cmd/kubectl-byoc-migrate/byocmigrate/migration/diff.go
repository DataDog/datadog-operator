// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package migration

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/pmezard/go-difflib/difflib"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/applyconfigurations"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/structured-merge-diff/v6/fieldpath"
	"sigs.k8s.io/yaml"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/cmd/kubectl-byoc-migrate/byocmigrate/converter"
	byocutils "github.com/DataDog/datadog-operator/internal/controller/datadogbyoccluster/utils"
)

// Diff compares the live objects with the objects after the migration, and returns a unified diff and the findings,
// which are what prevents the migration. It renders the resources of the live DatadogBYOCCluster, which adopt gives the
// objects to, or of file before the cluster is applied. The API server computes the objects after the migration with
// dry-run requests, so both sides have its defaults:
//   - an object that the cluster adopts is replaced with the resource of the cluster, because adopt gives the fields
//     of the object to the controller, whose apply then removes the fields that it does not set. A Service keeps the
//     fields of the managers other than the tools that apply the manifests (givesFields),
//   - a resource that the cluster or a Worker creates is created, and compared with the object of a manifest that it
//     replaces, if any,
//   - a resource that the cluster already controls, after adopt, is applied as the controller applies it.
//
// Diff writes nothing.
func (m *Migration) Diff(ctx context.Context, cloudprem converter.Manifest, pipelines []converter.PipelineManifest, file *datadoghqv1alpha1.DatadogBYOCCluster) (string, []converter.Finding, error) {
	cluster, err := m.getCluster(ctx)
	if err != nil {
		return "", nil, err
	}
	var plan *converter.Plan
	switch {
	case cluster != nil && file != nil:
		return "", nil, fmt.Errorf("DatadogBYOCCluster %s exists, and diff compares the objects with its resources; omit --from-file", m.Cluster)
	case cluster != nil:
		plan, err = m.livePlan(cluster, cloudprem, pipelines)
	case file != nil:
		plan, err = converter.NewPlan(file, cloudprem, pipelines)
	default:
		return "", nil, fmt.Errorf("DatadogBYOCCluster %s does not exist; pass the cluster that convert printed with --from-file", m.Cluster)
	}
	if err != nil {
		return "", nil, err
	}
	d := &differ{Migration: m, plan: plan, cluster: cluster, findings: plan.Findings()}
	for _, desired := range plan.Desired {
		if err := d.compare(ctx, plan.CloudPremObjects, desired); err != nil {
			return "", nil, err
		}
	}
	for _, pipeline := range plan.Pipelines {
		for _, desired := range pipeline.WorkerResources.Objects() {
			if err := d.compare(ctx, pipeline.Objects, desired); err != nil {
				return "", nil, err
			}
		}
	}
	for _, pipeline := range plan.NewPipelines {
		for _, desired := range pipeline.WorkerResources.Objects() {
			if err := d.compare(ctx, nil, desired); err != nil {
				return "", nil, err
			}
		}
	}
	d.writeNotAdopted()
	return d.diff.String(), d.findings, nil
}

type differ struct {
	*Migration
	plan     *converter.Plan
	cluster  *datadoghqv1alpha1.DatadogBYOCCluster
	diff     strings.Builder
	findings []converter.Finding
}

func (d *differ) addFinding(severity converter.Severity, object, message string) {
	d.findings = append(d.findings, converter.Finding{Severity: severity, Object: object, Message: message})
}

// compare compares desired after the migration with its live object, or with the live object of the manifest object
// that it replaces when desired does not exist yet.
func (d *differ) compare(ctx context.Context, manifestObjects []converter.PlannedObject, desired client.Object) error {
	planned := plannedFor(manifestObjects, desired)
	live, err := d.getLive(ctx, desired)
	if err != nil {
		return err
	}
	switch {
	case live == nil && planned.Action == converter.ActionAdopt:
		d.addFinding(converter.SeverityBlocker, "", d.missingError(d.plan, desired).Error())
		return nil
	case live == nil, d.createdByCluster(live):
	case planned.Action != converter.ActionAdopt:
		d.addFinding(converter.SeverityBlocker, d.name(desired), fmt.Sprintf("it exists, but it is not an object of manifest %s that the cluster adopts, so the controllers do not take it over", d.plan.CloudPrem.Name))
		return nil
	case !adoptable(live):
		d.addFinding(converter.SeverityBlocker, d.name(desired), "it "+errNotAdoptable.Error())
		return nil
	}
	before := live
	if live == nil && planned.Object != nil {
		if before, err = d.getLive(ctx, planned.Object); err != nil {
			return err
		}
	}
	after, err := d.afterMigration(ctx, live, desired)
	if err != nil {
		return d.refused(desired, err)
	}
	return d.writeDiff(before, after)
}

// refused reports a resource that the API server refuses as a blocker, so that diff goes on with the other resources.
func (d *differ) refused(desired client.Object, err error) error {
	d.addFinding(converter.SeverityBlocker, d.name(desired), "the API server refuses the resource of the cluster: "+err.Error())
	return nil
}

// createdByCluster reports whether the cluster, or one of its Workers, controls the live object, as after adopt.
func (d *differ) createdByCluster(live client.Object) bool {
	if d.cluster == nil {
		return false
	}
	if metav1.IsControlledBy(live, d.cluster) {
		return true
	}
	owner := metav1.GetControllerOfNoCopy(live)
	return owner != nil && owner.Kind == "DatadogObservabilityPipelinesWorker" && slices.ContainsFunc(d.plan.Workers(), func(worker converter.WorkerPlan) bool {
		return worker.Worker.Name == owner.Name
	})
}

// plannedFor returns the object of a manifest that desired adopts or replaces, if any.
func plannedFor(objects []converter.PlannedObject, desired client.Object) converter.PlannedObject {
	for _, planned := range objects {
		if planned.Desired == desired {
			return planned
		}
	}
	return converter.PlannedObject{}
}

// afterMigration returns the object that the API server stores once the migration applied desired over live, or
// creates desired when live is nil, without writing anything.
func (d *differ) afterMigration(ctx context.Context, live, desired client.Object) (client.Object, error) {
	after := desired.DeepCopyObject().(client.Object)
	gvk, err := apiutil.GVKForObject(after, d.Client.Scheme())
	if err != nil {
		return nil, err
	}
	after.GetObjectKind().SetGroupVersionKind(gvk)
	switch {
	case live == nil:
		err = d.Client.Create(ctx, after, client.DryRunAll)
	case d.createdByCluster(live):
		err = d.Client.Patch(ctx, after, client.Apply, client.ForceOwnership, client.FieldOwner(byocutils.ClusterFieldOwner), client.DryRunAll)
	default:
		if after, err = d.withKeptFields(live, after, gvk); err != nil {
			return nil, err
		}
		after.SetResourceVersion(live.GetResourceVersion())
		keepReplicas(live, after)
		err = d.Client.Update(ctx, after, client.DryRunAll)
	}
	return after, err
}

// withKeptFields returns desired with the fields of live that adopt does not give to the controller (givesFields),
// such as the load balancer settings of a Service, because the apply of the controller leaves them.
func (d *differ) withKeptFields(live, desired client.Object, gvk schema.GroupVersionKind) (client.Object, error) {
	kept := &fieldpath.Set{}
	for _, entry := range live.GetManagedFields() {
		if entry.Subresource != "" || entry.FieldsV1 == nil || givesFields(live, entry, byocutils.ClusterFieldOwner) {
			continue
		}
		fields := &fieldpath.Set{}
		if err := fields.FromJSON(entry.FieldsV1.GetRawReader()); err != nil {
			return nil, fmt.Errorf("decode fields of manager %s: %w", entry.Manager, err)
		}
		kept = kept.Union(fields)
	}
	if kept.Empty() {
		return desired, nil
	}
	converter := applyconfigurations.NewTypeConverter(d.Client.Scheme())
	live = live.DeepCopyObject().(client.Object)
	live.GetObjectKind().SetGroupVersionKind(gvk)
	liveValue, err := converter.ObjectToTyped(live)
	if err != nil {
		return nil, err
	}
	desiredValue, err := converter.ObjectToTyped(desired)
	if err != nil {
		return nil, err
	}
	merged, err := liveValue.ExtractItems(kept).Merge(desiredValue)
	if err != nil {
		return nil, err
	}
	object, err := converter.TypedToObject(merged)
	if err != nil {
		return nil, err
	}
	typed, err := d.Client.Scheme().New(gvk)
	if err != nil {
		return nil, err
	}
	if err = runtime.DefaultUnstructuredConverter.FromUnstructured(object.(*unstructured.Unstructured).Object, typed); err != nil {
		return nil, err
	}
	return typed.(client.Object), nil
}

// keepReplicas keeps the replicas of a live workload when the resource of the cluster does not set them, because adopt
// leaves them to the HorizontalPodAutoscaler that scales the workload.
func keepReplicas(live, after client.Object) {
	switch after := after.(type) {
	case *appsv1.StatefulSet:
		if after.Spec.Replicas == nil {
			after.Spec.Replicas = live.(*appsv1.StatefulSet).Spec.Replicas
		}
	case *appsv1.Deployment:
		if after.Spec.Replicas == nil {
			after.Spec.Replicas = live.(*appsv1.Deployment).Spec.Replicas
		}
	}
}

// writeDiff writes the unified diff from before, a live object or nil, to the object after the migration.
func (d *differ) writeDiff(before, after client.Object) error {
	afterName := d.name(after)
	beforeText, fromFile := "", afterName+" (absent)"
	if before != nil {
		fromFile = d.name(before) + " (live)"
		var err error
		if beforeText, err = d.normalizedYAML(before); err != nil {
			return err
		}
	}
	afterText, err := d.normalizedYAML(after)
	if err != nil {
		return err
	}
	text, err := difflib.GetUnifiedDiffString(difflib.UnifiedDiff{
		A:        difflib.SplitLines(beforeText),
		B:        difflib.SplitLines(afterText),
		FromFile: fromFile,
		ToFile:   afterName + " (after the migration)",
		Context:  3,
	})
	if err != nil {
		return fmt.Errorf("diff %s: %w", afterName, err)
	}
	d.diff.WriteString(text)
	return nil
}

// normalizedYAML returns the object with its name, labels, annotations, and spec or data, which the migration sets.
func (d *differ) normalizedYAML(object client.Object) (string, error) {
	content, err := runtime.DefaultUnstructuredConverter.ToUnstructured(object)
	if err != nil {
		return "", err
	}
	delete(content, "status")
	metadata, _ := content["metadata"].(map[string]any)
	kept := map[string]any{"name": object.GetName()}
	for _, field := range []string{"labels", "annotations"} {
		if value, found := metadata[field]; found {
			kept[field] = value
		}
	}
	content["metadata"] = kept
	sortEnvironments(content)
	text, err := yaml.Marshal(content)
	return string(text), err
}

// sortEnvironments sorts the environment variables of the containers by name, so that the diff does not show a variable
// that only moves as removed and added.
func sortEnvironments(value any) {
	switch value := value.(type) {
	case map[string]any:
		for key, nested := range value {
			if env, isList := nested.([]any); isList && key == "env" {
				slices.SortStableFunc(env, func(a, b any) int { return strings.Compare(envName(a), envName(b)) })
			}
			sortEnvironments(nested)
		}
	case []any:
		for _, item := range value {
			sortEnvironments(item)
		}
	}
}

func envName(variable any) string {
	fields, _ := variable.(map[string]any)
	name, _ := fields["name"].(string)
	return name
}

// writeNotAdopted lists the objects of the manifests that the cluster does not adopt, which the diff does not show as
// such: the objects that the cluster replaces, and the objects that the migration leaves as is.
func (d *differ) writeNotAdopted() {
	objects := slices.Clone(d.plan.CloudPremObjects)
	for _, pipeline := range d.plan.Pipelines {
		objects = append(objects, pipeline.Objects...)
	}
	for _, group := range []struct {
		action converter.Action
		header string
	}{
		{converter.ActionReplace, "# The cluster replaces these objects of the manifests; delete them after the migration:"},
		{converter.ActionKeep, "# The migration leaves these objects of the manifests as is:"},
	} {
		var names []string
		for _, planned := range objects {
			if planned.Action == group.action {
				names = append(names, d.name(planned.Object))
			}
		}
		if len(names) == 0 {
			continue
		}
		fmt.Fprintln(&d.diff, group.header)
		for _, name := range names {
			fmt.Fprintf(&d.diff, "#   %s\n", name)
		}
	}
}
