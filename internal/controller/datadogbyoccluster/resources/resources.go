// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package resources

import (
	"crypto/sha256"
	"encoding/hex"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	byocimage "github.com/DataDog/datadog-operator/internal/controller/datadogbyoccluster/image"
)

// Resources is the complete set of Kubernetes resources managed for a cluster.
type Resources struct {
	configMap       *corev1.ConfigMap
	serviceAccount  *corev1.ServiceAccount
	headlessService *corev1.Service
	components      map[string]*ComponentResources
	objects         []client.Object
	obsoleteObjects []client.Object
}

// ComponentResources contains the resources managed for a component.
// Exactly one of Deployment and StatefulSet is set.
type ComponentResources struct {
	Service             *corev1.Service
	Deployment          *appsv1.Deployment
	StatefulSet         *appsv1.StatefulSet
	HPA                 *autoscalingv2.HorizontalPodAutoscaler
	PodDisruptionBudget *policyv1.PodDisruptionBudget
}

// Objects returns the resources in apply order.
func (r *Resources) Objects() []client.Object {
	return r.objects
}

// ObsoleteObjects returns optional resources that are no longer desired.
func (r *Resources) ObsoleteObjects() []client.Object {
	return r.obsoleteObjects
}

// Component returns the resources of an enabled component, or nil when the component is disabled.
func (r *Resources) Component(name string) *ComponentResources {
	return r.components[name]
}

// BuildResources builds deterministic Kubernetes resources from a defaulted cluster and resolved images.
func BuildResources(cluster *datadoghqv1alpha1.DatadogBYOCCluster, images *byocimage.ResolvedImages) (*Resources, error) {
	cluster = cluster.DeepCopy()

	configMap, err := newConfigMap(cluster)
	if err != nil {
		return nil, err
	}
	r := &Resources{
		configMap:       configMap,
		headlessService: newHeadlessService(cluster),
		components:      map[string]*ComponentResources{},
	}
	r.objects = append(r.objects, r.configMap)
	if _, managed := serviceAccountName(cluster); managed {
		r.serviceAccount = newServiceAccount(cluster)
		r.objects = append(r.objects, r.serviceAccount)
	}
	r.objects = append(r.objects, r.headlessService)

	checksum := sha256.Sum256([]byte(configMap.Data[nodeConfigFileName]))
	pod := podInput{cluster: cluster, image: images.Pomsky, checksum: hex.EncodeToString(checksum[:])}
	for _, c := range components(cluster) {
		if !c.enabled() {
			r.obsoleteObjects = append(r.obsoleteObjects, disabledComponentObjects(cluster, c)...)
			continue
		}
		resources, err := newComponentResources(pod, c)
		if err != nil {
			return nil, err
		}
		r.components[c.name] = resources
		r.objects = append(r.objects, resources.objects()...)
		r.obsoleteObjects = append(r.obsoleteObjects, resources.obsoleteObjects(cluster, c)...)
	}
	return r, nil
}

func (r *ComponentResources) objects() []client.Object {
	objects := []client.Object{r.Service}
	if r.Deployment != nil {
		objects = append(objects, r.Deployment)
	}
	if r.StatefulSet != nil {
		objects = append(objects, r.StatefulSet)
	}
	if r.HPA != nil {
		objects = append(objects, r.HPA)
	}
	if r.PodDisruptionBudget != nil {
		objects = append(objects, r.PodDisruptionBudget)
	}
	return objects
}

func (r *ComponentResources) obsoleteObjects(cluster *datadoghqv1alpha1.DatadogBYOCCluster, c component) []client.Object {
	metadata := componentObjectMeta(cluster, c.name)
	var objects []client.Object
	if c.stateful != nil && r.HPA == nil {
		objects = append(objects, &autoscalingv2.HorizontalPodAutoscaler{ObjectMeta: metadata})
	}
	if r.PodDisruptionBudget == nil {
		objects = append(objects, &policyv1.PodDisruptionBudget{ObjectMeta: metadata})
	}
	return objects
}

// disabledComponentObjects returns the resources of a disabled component. Only Deployment components are optional.
func disabledComponentObjects(cluster *datadoghqv1alpha1.DatadogBYOCCluster, c component) []client.Object {
	metadata := componentObjectMeta(cluster, c.name)
	return []client.Object{
		&corev1.Service{ObjectMeta: metadata},
		&appsv1.Deployment{ObjectMeta: metadata},
		&policyv1.PodDisruptionBudget{ObjectMeta: metadata},
	}
}

func componentObjectMeta(cluster *datadoghqv1alpha1.DatadogBYOCCluster, componentName string) metav1.ObjectMeta {
	return metav1.ObjectMeta{Name: ComponentResourceName(cluster.Name, componentName), Namespace: cluster.Namespace}
}
