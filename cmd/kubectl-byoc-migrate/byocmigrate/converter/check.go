// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package converter

import (
	"fmt"
	"slices"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	byocvalidation "github.com/DataDog/datadog-operator/internal/controller/datadogbyoccluster/validation"
)

// Severity tells whether a finding prevents the migration.
type Severity string

const (
	// SeverityBlocker means that the controllers cannot adopt the resources, or that adopting them breaks the workloads.
	SeverityBlocker Severity = "Blocker"
	// SeverityWarning means that the migration needs attention that the diff of the objects does not show.
	SeverityWarning Severity = "Warning"
)

// Finding is something that the converted cluster cannot express.
type Finding struct {
	Severity Severity
	// Object is the kind and name of the object that the finding is about, if any.
	Object  string
	Message string
}

func (f Finding) String() string {
	if f.Object == "" {
		return fmt.Sprintf("%s: %s", f.Severity, f.Message)
	}
	return fmt.Sprintf("%s: %s: %s", f.Severity, f.Object, f.Message)
}

// Findings reports what prevents the migration: the fields that the manifests do not provide, an invalid cluster, and
// the objects of the manifests that block it. It also reports what needs attention that the diff of the objects does
// not show. The diff shows the settings of the manifests that the cluster loses or changes.
func (p *Plan) Findings() []Finding {
	cluster := p.Cluster
	findings := requiredFindings(cluster)
	for _, object := range p.CloudPrem.Objects {
		if object.GetNamespace() != cluster.Namespace {
			findings = append(findings, Finding{Severity: SeverityBlocker, Object: objectName(object), Message: fmt.Sprintf("the manifest puts it in namespace %s, but the cluster is in namespace %s; run the command in the namespace of the manifest", object.GetNamespace(), cluster.Namespace)})
		}
	}
	for _, err := range byocvalidation.ValidateClusterSpec(&p.rendered.defaulted.Spec) {
		findings = append(findings, Finding{Severity: SeverityBlocker, Message: "the converted cluster is invalid: " + err.Error()})
	}
	findings = append(findings, planFindings(p)...)
	if cluster.Spec.Identity != nil && cluster.Spec.Provider != nil && cluster.Spec.Provider.AWS != nil && cluster.Spec.Provider.AWS.IRSARoleARN != nil {
		findings = append(findings, Finding{Severity: SeverityWarning, Message: "the controller sets the IRSA role only on the ServiceAccount it manages, not on " + *cluster.Spec.Identity.ServiceAccountName})
	}
	return findings
}

// requiredFindings reports the fields that the CRD requires but the manifests do not provide.
func requiredFindings(cluster *datadoghqv1alpha1.DatadogBYOCCluster) []Finding {
	var findings []Finding
	if cluster.Spec.Datadog.APIKeySecretRef == nil {
		findings = append(findings, Finding{Severity: SeverityBlocker, Message: "the API key does not come from a Secret; create one and set spec.datadog.apiKeySecretRef"})
	}
	components := cluster.Spec.Components
	if components.Metastore.Database == nil {
		findings = append(findings, Finding{Severity: SeverityBlocker, Message: "QW_METASTORE_URI of the metastore does not come from a Secret; create one and set spec.components.metastore.database.uriSecretRef"})
	}
	if components.ReadOnlyMetastore != nil && components.ReadOnlyMetastore.Database == nil {
		findings = append(findings, Finding{Severity: SeverityBlocker, Message: "QW_METASTORE_READ_REPLICA_URI of the read-only metastore does not come from a Secret; create one and set spec.components.readOnlyMetastore.database.uriSecretRef"})
	}
	for _, pipeline := range components.Pipelines {
		if pipeline.PipelineID == nil {
			findings = append(findings, Finding{Severity: SeverityBlocker, Message: fmt.Sprintf("pipeline %s: the manifest sets no DD_OP_PIPELINE_ID", pipeline.Name)})
		}
		if len(pipeline.Ports) == 0 {
			findings = append(findings, Finding{Severity: SeverityBlocker, Message: fmt.Sprintf("pipeline %s: the manifest exposes no source port; set spec.components.pipelines[].ports", pipeline.Name)})
		}
	}
	return findings
}

// planFindings reports the objects of the manifests that prevent the migration, or that it changes unexpectedly.
func planFindings(plan *Plan) []Finding {
	var findings []Finding
	for _, planned := range plan.CloudPremObjects {
		switch planned.Action {
		case ActionAdopt:
			findings = append(findings, immutableFindings(planned.Object, planned.Desired)...)
		case ActionBlock:
			findings = append(findings, Finding{Severity: SeverityBlocker, Object: objectName(planned.Object), Message: planned.Reason})
		}
	}
	for _, pipeline := range plan.Pipelines {
		findings = append(findings, pipelineFindings(pipeline)...)
	}
	return findings
}

// pipelineFindings reports what prevents the Worker of a pipeline from replacing its manifest, or loses its data.
func pipelineFindings(pipeline PipelinePlan) []Finding {
	owner := "pipeline " + pipeline.Pipeline
	var findings []Finding
	if pipeline.StatefulSet.Spec.Template.Labels[instanceLabel] == pipeline.Worker.Name {
		findings = append(findings, Finding{Severity: SeverityBlocker, Message: fmt.Sprintf("%s: the Pods of manifest %s have the %s label %s of the Worker, so the Services, PodDisruptionBudget, and HorizontalPodAutoscaler of the Worker would also select them; rename the pipeline", owner, pipeline.Manifest.Name, instanceLabel, pipeline.Worker.Name)})
	}
	if policy := pipeline.StatefulSet.Spec.PersistentVolumeClaimRetentionPolicy; len(pipeline.StatefulSet.Spec.VolumeClaimTemplates) > 0 && policy != nil && policy.WhenScaled == appsv1.DeletePersistentVolumeClaimRetentionPolicyType {
		findings = append(findings, Finding{Severity: SeverityWarning, Message: fmt.Sprintf("%s: scaling StatefulSet/%s to zero deletes its PersistentVolumeClaims and the data they hold", owner, pipeline.StatefulSet.Name)})
	}
	return findings
}

// referencedObjects returns the objects that the converted cluster uses without managing them.
func referencedObjects(cluster *datadoghqv1alpha1.DatadogBYOCCluster) []string {
	var referenced []string
	if identity := cluster.Spec.Identity; identity != nil && identity.ServiceAccountName != nil {
		referenced = append(referenced, "ServiceAccount/"+*identity.ServiceAccountName)
	}
	if secret := cluster.Spec.Datadog.APIKeySecretRef; secret != nil {
		referenced = append(referenced, "Secret/"+secret.Name)
	}
	components := cluster.Spec.Components
	for _, metastore := range []*datadoghqv1alpha1.DatadogBYOCClusterMetastoreComponentSpec{components.Metastore, components.ReadOnlyMetastore} {
		if metastore != nil && metastore.Database != nil && metastore.Database.URISecretRef != nil {
			referenced = append(referenced, "Secret/"+metastore.Database.URISecretRef.Name)
		}
	}
	return referenced
}

// immutableDifferences reports the fields of a workload that the API server refuses to change, and that differ
// between current and the desired object. The volume claim templates are left to the caller, because a live
// StatefulSet holds them after defaulting.
func immutableDifferences(current, desired client.Object) []string {
	var differences []string
	switch current := current.(type) {
	case *appsv1.StatefulSet:
		desired := desired.(*appsv1.StatefulSet)
		differences = append(differences,
			difference("spec.selector", metav1.FormatLabelSelector(current.Spec.Selector), metav1.FormatLabelSelector(desired.Spec.Selector)),
			difference("spec.serviceName", current.Spec.ServiceName, desired.Spec.ServiceName),
			difference("spec.podManagementPolicy", string(current.Spec.PodManagementPolicy), string(desired.Spec.PodManagementPolicy)),
		)
	case *appsv1.Deployment:
		desired := desired.(*appsv1.Deployment)
		differences = append(differences, difference("spec.selector", metav1.FormatLabelSelector(current.Spec.Selector), metav1.FormatLabelSelector(desired.Spec.Selector)))
	}
	return slices.DeleteFunc(differences, func(difference string) bool { return difference == "" })
}

// immutableFindings reports the differences that prevent the adoption or break the traffic of the workloads.
func immutableFindings(chart, controller client.Object) []Finding {
	differences := immutableDifferences(chart, controller)
	switch chart := chart.(type) {
	case *appsv1.StatefulSet:
		if !apiequality.Semantic.DeepEqual(chart.Spec.VolumeClaimTemplates, controller.(*appsv1.StatefulSet).Spec.VolumeClaimTemplates) {
			differences = append(differences, "spec.volumeClaimTemplates differ")
		}
	case *corev1.Service:
		// Changing the type of a LoadBalancer Service deletes its load balancer.
		if difference := difference("spec.type", string(serviceType(chart)), string(serviceType(controller.(*corev1.Service)))); difference != "" {
			differences = append(differences, difference)
		}
	}
	findings := make([]Finding, 0, len(differences))
	for _, difference := range differences {
		findings = append(findings, Finding{Severity: SeverityBlocker, Object: objectName(chart), Message: difference})
	}
	return findings
}

func difference(path, chart, controller string) string {
	if chart == controller {
		return ""
	}
	return fmt.Sprintf("%s is %q in the manifest, but the controller sets %q", path, chart, controller)
}

func serviceType(service *corev1.Service) corev1.ServiceType {
	if service.Spec.Type == "" {
		return corev1.ServiceTypeClusterIP
	}
	return service.Spec.Type
}

func sameKind(a, b client.Object) bool {
	return objectKind(a) == objectKind(b)
}

func objectKind(object client.Object) string {
	if kind := object.GetObjectKind().GroupVersionKind().Kind; kind != "" {
		return kind
	}
	gvk, err := apiutil.GVKForObject(object, scheme.Scheme)
	if err != nil {
		return fmt.Sprintf("%T", object)
	}
	return gvk.Kind
}

func objectName(object client.Object) string {
	return objectKind(object) + "/" + object.GetName()
}
