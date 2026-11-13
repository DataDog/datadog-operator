// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package converter

import (
	"fmt"
	"maps"
	"slices"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	byocresources "github.com/DataDog/datadog-operator/internal/controller/datadogbyoccluster/resources"
	workerresources "github.com/DataDog/datadog-operator/internal/controller/datadogobservabilitypipelinesworker/resources"
)

// Action is what the migration does with an object of a manifest.
type Action string

const (
	// ActionAdopt marks a CloudPrem object that the cluster adopts in place, because the cluster applies a resource
	// with the same kind and name.
	ActionAdopt Action = "Adopt"
	// ActionReplace marks an object that a resource of the cluster replaces, which can be deleted after the migration.
	ActionReplace Action = "Replace"
	// ActionKeep marks an object that the migration leaves as is.
	ActionKeep Action = "Keep"
	// ActionBlock marks an object that prevents the migration.
	ActionBlock Action = "Block"
)

// PlannedObject is an object of a manifest and what the migration does with it.
type PlannedObject struct {
	// Object is the object as rendered in the manifest of the manifest.
	Object client.Object
	Action Action
	// Desired is the resource with the same kind and name that the cluster applies, for an adopted object, or the
	// resource of the same kind that replaces a replaced object, if any.
	Desired client.Object
	// Reason explains the action.
	Reason string
}

// WorkerPlan is the Worker that the cluster creates for a pipeline.
type WorkerPlan struct {
	Pipeline string
	// Worker is the Worker that the cluster creates for the pipeline.
	Worker *datadoghqv1alpha1.DatadogObservabilityPipelinesWorker
	// WorkerResources are the resources that the Worker controller creates for Worker.
	WorkerResources *workerresources.Resources
}

// PipelinePlan is how the Worker of a pipeline replaces an Observability Pipelines Worker manifest.
type PipelinePlan struct {
	WorkerPlan
	Manifest Manifest
	// StatefulSet is the StatefulSet of the manifest.
	StatefulSet *appsv1.StatefulSet
	Objects     []PlannedObject
}

// Plan is what the migration does with each object of the manifests.
type Plan struct {
	Cluster *datadoghqv1alpha1.DatadogBYOCCluster
	// Desired are the resources that the DatadogBYOCCluster controller applies, without the Workers.
	Desired          []client.Object
	CloudPrem        Manifest
	CloudPremObjects []PlannedObject
	Pipelines        []PipelinePlan
	// NewPipelines are the pipelines of the cluster that replace no manifest, whose Workers start without traffic.
	NewPipelines []WorkerPlan

	rendered *rendered
}

// NewPlan renders the resources of the cluster and decides what the migration does with each object of the
// manifests. The cluster must be named after the Helm fullname and be in the namespace of the CloudPrem manifest, and the
// pipelines of the manifests must be pipelines of the cluster.
func NewPlan(cluster *datadoghqv1alpha1.DatadogBYOCCluster, cloudprem Manifest, pipelines []PipelineManifest) (*Plan, error) {
	name, err := ClusterName(cloudprem)
	if err != nil {
		return nil, err
	}
	if cluster.Name != name {
		// The cluster would not adopt the workloads of the manifest, and would run a second CloudPrem next to them.
		return nil, fmt.Errorf("DatadogBYOCCluster %s must be named %s, after the Helm fullname of manifest %s", cluster.Name, name, cloudprem.Name)
	}
	if cluster.Namespace != cloudprem.Namespace {
		// The controller would not find the objects of the manifest, and would create a second CloudPrem next to them.
		return nil, fmt.Errorf("DatadogBYOCCluster %s is in namespace %q, but manifest %s is in namespace %q; the cluster must be in the namespace of the manifest", cluster.Name, cluster.Namespace, cloudprem.Name, cloudprem.Namespace)
	}
	result, err := render(cluster)
	if err != nil {
		return nil, err
	}
	referenced := referencedObjects(cluster)
	plan := &Plan{
		Cluster:          cluster,
		Desired:          result.cluster.Objects(),
		CloudPrem:        cloudprem,
		CloudPremObjects: planCloudPrem(cloudprem.Objects, result.cluster, referenced, cluster.Name),
		rendered:         result,
	}
	for _, pipeline := range pipelines {
		pipelinePlan, err := planPipeline(pipeline, result, referenced)
		if err != nil {
			return nil, err
		}
		plan.Pipelines = append(plan.Pipelines, pipelinePlan)
	}
	for _, spec := range cluster.Spec.Components.Pipelines {
		if slices.ContainsFunc(pipelines, func(pipeline PipelineManifest) bool { return pipeline.Pipeline == spec.Name }) {
			continue
		}
		plan.NewPipelines = append(plan.NewPipelines, WorkerPlan{Pipeline: spec.Name, Worker: result.workers[spec.Name], WorkerResources: result.workerResources[spec.Name]})
	}
	return plan, nil
}

// Workers returns the Workers of every pipeline of the cluster.
func (p *Plan) Workers() []WorkerPlan {
	workers := make([]WorkerPlan, 0, len(p.Pipelines)+len(p.NewPipelines))
	for _, pipeline := range p.Pipelines {
		workers = append(workers, pipeline.WorkerPlan)
	}
	return append(workers, p.NewPipelines...)
}

// ObjectsWith returns the objects of the CloudPrem manifest and of the pipelines with the action.
func (p *Plan) ObjectsWith(action Action) []PlannedObject {
	objects := slices.Clone(p.CloudPremObjects)
	for _, pipeline := range p.Pipelines {
		objects = append(objects, pipeline.Objects...)
	}
	return slices.DeleteFunc(objects, func(object PlannedObject) bool { return object.Action != action })
}

func planCloudPrem(objects []client.Object, resources *byocresources.Resources, referenced []string, clusterName string) []PlannedObject {
	desired := resources.Objects()
	planned := make([]PlannedObject, len(objects))
	var adoptedWorkloads []client.Object
	for i, object := range objects {
		planned[i] = PlannedObject{Object: object, Action: ActionKeep, Reason: "the controllers do not manage this object, so it is left as is; delete it after the migration if it is no longer needed"}
		if counterpart, found := findObject(desired, func(rendered client.Object) bool {
			return sameKind(rendered, object) && rendered.GetName() == object.GetName()
		}); found {
			planned[i] = PlannedObject{Object: object, Action: ActionAdopt, Desired: counterpart, Reason: "the cluster adopts this object"}
			if podTemplateLabels(object) != nil {
				adoptedWorkloads = append(adoptedWorkloads, object)
			}
		}
	}
	readOnlyMetastore := byocresources.ComponentResourceName(clusterName, byocresources.ReadOnlyMetastoreComponentName)
	for i, object := range objects {
		if planned[i].Action == ActionAdopt {
			continue
		}
		switch {
		case selectsPodsOf(object, readOnlyMetastoreComponent):
			planned[i].Action = ActionReplace
			planned[i].Desired = replacementOf(object, readOnlyMetastoreResources(resources)...)
			planned[i].Reason = fmt.Sprintf("the cluster replaces the read-only metastore with Deployment/%s, which runs next to this object; delete it after the migration", readOnlyMetastore)
		case isBudgetOf(object, adoptedWorkloads):
			planned[i].Action = ActionReplace
			planned[i].Reason = "the cluster sets no PodDisruptionBudget for the Pods that this object selects; delete it after the migration"
		case scaledWorkload(object, adoptedWorkloads) != "":
			planned[i].Action = ActionBlock
			planned[i].Reason = fmt.Sprintf("it scales %s, which the cluster does not autoscale, so it would fight the replicas that the controller sets; set the autoscaling of the component or delete this object before the migration", scaledWorkload(object, adoptedWorkloads))
		case slices.Contains(referenced, objectName(object)):
			planned[i].Reason = "the controllers do not manage this object, but the converted cluster uses it; keep it after the migration"
		}
	}
	return planned
}

func planPipeline(pipeline PipelineManifest, result *rendered, referenced []string) (PipelinePlan, error) {
	worker, found := result.workers[pipeline.Pipeline]
	if !found {
		return PipelinePlan{}, fmt.Errorf("the cluster has no pipeline %s for manifest %s", pipeline.Pipeline, pipeline.Manifest.Name)
	}
	statefulSet, err := WorkerStatefulSet(pipeline.Manifest)
	if err != nil {
		return PipelinePlan{}, err
	}
	plan := PipelinePlan{
		WorkerPlan:  WorkerPlan{Pipeline: pipeline.Pipeline, Worker: worker, WorkerResources: result.workerResources[pipeline.Pipeline]},
		Manifest:    pipeline.Manifest,
		StatefulSet: statefulSet,
	}

	replaced := fmt.Sprintf("Worker %s replaces this object; delete it after the migration", worker.Name)
	replacements := plan.WorkerResources.Objects()
	for _, object := range pipeline.Manifest.Objects {
		planned := PlannedObject{Object: object, Action: ActionKeep, Reason: "the controllers do not manage this object, so it is left as is; delete it after the migration if it is no longer needed"}
		switch typed := object.(type) {
		case *corev1.Service:
			if maps.Equal(typed.Spec.Selector, statefulSet.Spec.Selector.MatchLabels) {
				planned.Action = ActionReplace
				planned.Reason = fmt.Sprintf("Worker %s replaces this Service; move its clients to the Worker, then delete it", worker.Name)
				// The Worker has no headless Service: its StatefulSet uses its only Service.
				if typed.Spec.ClusterIP != corev1.ClusterIPNone {
					planned.Desired = replacementOf(object, replacements...)
				}
			}
		case *appsv1.StatefulSet:
			planned.Action = ActionReplace
			planned.Desired = replacementOf(object, replacements...)
			planned.Reason = fmt.Sprintf("Worker %s replaces this StatefulSet; once its clients send to the Worker, scale it to zero, then delete it", worker.Name)
		case *autoscalingv2.HorizontalPodAutoscaler, *policyv1.PodDisruptionBudget, *corev1.ServiceAccount, *corev1.ConfigMap:
			planned.Action = ActionReplace
			planned.Desired = replacementOf(object, replacements...)
			planned.Reason = replaced
		case *corev1.Secret:
			if slices.Contains(referenced, objectName(object)) {
				planned.Reason = "the controllers do not manage this object, but the converted cluster uses it; keep it after the migration"
			} else {
				planned.Action = ActionReplace
				planned.Reason = replaced
			}
		}
		plan.Objects = append(plan.Objects, planned)
	}
	return plan, nil
}

// replacementOf returns the resource of the kind of object among the resources that replace it, or nil.
func replacementOf(object client.Object, replacements ...client.Object) client.Object {
	replacement, _ := findObject(replacements, func(replacement client.Object) bool { return sameKind(replacement, object) })
	return replacement
}

// readOnlyMetastoreResources returns the resources that the cluster renders for its read-only metastore.
func readOnlyMetastoreResources(resources *byocresources.Resources) []client.Object {
	component := resources.Component(byocresources.ReadOnlyMetastoreComponentName)
	if component == nil {
		return nil
	}
	var objects []client.Object
	if component.Service != nil {
		objects = append(objects, component.Service)
	}
	if component.Deployment != nil {
		objects = append(objects, component.Deployment)
	}
	if component.HPA != nil {
		objects = append(objects, component.HPA)
	}
	if component.PodDisruptionBudget != nil {
		objects = append(objects, component.PodDisruptionBudget)
	}
	return objects
}

// podTemplateLabels returns the Pod labels of a workload, or nil for other objects.
func podTemplateLabels(object client.Object) map[string]string {
	switch workload := object.(type) {
	case *appsv1.Deployment:
		return workload.Spec.Template.Labels
	case *appsv1.StatefulSet:
		return workload.Spec.Template.Labels
	default:
		return nil
	}
}

// selectsPodsOf reports whether a workload, Service, or PodDisruptionBudget of the chart selects the Pods of a component.
func selectsPodsOf(object client.Object, component string) bool {
	switch typed := object.(type) {
	case *appsv1.Deployment:
		return selects(typed.Spec.Selector, component)
	case *appsv1.StatefulSet:
		return selects(typed.Spec.Selector, component)
	case *corev1.Service:
		return typed.Spec.Selector[componentLabel] == component
	case *policyv1.PodDisruptionBudget:
		return selects(typed.Spec.Selector, component)
	default:
		return false
	}
}

// isBudgetOf reports whether object is a PodDisruptionBudget that selects the Pods of one of the workloads.
func isBudgetOf(object client.Object, workloads []client.Object) bool {
	budget, ok := object.(*policyv1.PodDisruptionBudget)
	if !ok || budget.Spec.Selector == nil {
		return false
	}
	selector, err := metav1.LabelSelectorAsSelector(budget.Spec.Selector)
	if err != nil {
		return false
	}
	return slices.ContainsFunc(workloads, func(workload client.Object) bool {
		return selector.Matches(labels.Set(podTemplateLabels(workload)))
	})
}

// scaledWorkload returns the name of the workload that object scales when object is a HorizontalPodAutoscaler of one
// of the workloads, or an empty string.
func scaledWorkload(object client.Object, workloads []client.Object) string {
	hpa, ok := object.(*autoscalingv2.HorizontalPodAutoscaler)
	if !ok {
		return ""
	}
	for _, workload := range workloads {
		if scales(hpa, workload) {
			return objectName(workload)
		}
	}
	return ""
}

func scales(hpa *autoscalingv2.HorizontalPodAutoscaler, workload client.Object) bool {
	return hpa.Spec.ScaleTargetRef.Kind == objectKind(workload) && hpa.Spec.ScaleTargetRef.Name == workload.GetName()
}
