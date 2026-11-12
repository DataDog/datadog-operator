// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build integration
// +build integration

package controller

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/cmd/kubectl-byoc-migrate/byocmigrate/converter"
	"github.com/DataDog/datadog-operator/cmd/kubectl-byoc-migrate/byocmigrate/migration"
)

const (
	helmFieldManager = "helm"
	// migrationTimeout bounds the steps of a migration, which wait for several reconciles.
	migrationTimeout = time.Minute
)

var _ = Describe("Migration from the CloudPrem and Worker Helm charts", func() {
	var (
		namespace *corev1.Namespace
		cloudprem converter.Manifest
		pipelines []converter.PipelineManifest
		converted *datadoghqv1alpha1.DatadogBYOCCluster
		m         *migration.Migration
	)

	// applyCluster creates the converted cluster, as a deployment pipeline does before adopt.
	applyCluster := func() {
		Expect(k8sClient.Create(context.Background(), converted.DeepCopy())).To(Succeed())
	}
	// adopt runs adopt as the command does, with the plan of the live cluster.
	adopt := func() error {
		livePlan, err := m.LivePlan(context.Background(), cloudprem, pipelines)
		if err != nil {
			return err
		}
		return m.Adopt(context.Background(), livePlan)
	}

	BeforeEach(func() {
		namespace = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{GenerateName: "byoc-migration-test-"}}
		createKubernetesObject(k8sClient, namespace)

		cloudprem = installTestManifest(namespace.Name, "cloudprem", "cloudprem-full.yaml")
		pipelines = []converter.PipelineManifest{{Pipeline: "logs", Manifest: installTestManifest(namespace.Name, "logs", "opw-full.yaml")}}
		result, err := converter.Convert(cloudprem, pipelines)
		Expect(err).NotTo(HaveOccurred())
		converted = result.Cluster
		m = &migration.Migration{Client: k8sClient, Cluster: client.ObjectKeyFromObject(result.Cluster), Out: GinkgoWriter}
	})

	AfterEach(func() {
		deleteKubernetesObject(k8sClient, namespace)
	})

	It("adopts the CloudPrem resources in place and creates their replacements next to the manifests", func() {
		indexer := &appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Name: "cloudprem-indexer", Namespace: namespace.Name}}
		By("scaling the indexer as its HPA does")
		scaleTestWorkload(indexer, 3)
		applyCluster()

		Expect(adopt()).To(Succeed())

		cluster := &datadoghqv1alpha1.DatadogBYOCCluster{}
		Expect(k8sClient.Get(context.Background(), client.ObjectKey{Name: "cloudprem", Namespace: namespace.Name}, cluster)).To(Succeed())

		By("giving every field of the indexer to the cluster, apart from the replicas of its HPA")
		Eventually(func(g Gomega) {
			current := &appsv1.StatefulSet{}
			g.Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(indexer), current)).To(Succeed())
			g.Expect(metav1.IsControlledBy(current, cluster)).To(BeTrue())
			g.Expect(current.Labels).NotTo(HaveKey("helm.sh/chart"))
			g.Expect(current.Spec.Replicas).To(Equal(ptr.To[int32](3)))
			g.Expect(testManagers(current)).NotTo(ContainElement(helmFieldManager))
			g.Expect(testManagers(current)).To(ContainElement("hpa/scale"))
		}, timeout, interval).Should(Succeed())

		By("giving the fields of the chart on the Services to the cluster")
		Eventually(func(g Gomega) {
			service := &corev1.Service{}
			g.Expect(k8sClient.Get(context.Background(), client.ObjectKey{Name: "cloudprem-indexer", Namespace: namespace.Name}, service)).To(Succeed())
			g.Expect(metav1.IsControlledBy(service, cluster)).To(BeTrue())
			g.Expect(service.Labels).NotTo(HaveKey("helm.sh/chart"))
			g.Expect(testManagers(service)).NotTo(ContainElement(helmFieldManager))
		}, timeout, interval).Should(Succeed())

		By("creating the read-only metastore next to the one of the chart")
		Eventually(func(g Gomega) {
			current := &appsv1.Deployment{}
			g.Expect(k8sClient.Get(context.Background(), client.ObjectKey{Name: "cloudprem-read-only-metastore", Namespace: namespace.Name}, current)).To(Succeed())
			g.Expect(metav1.IsControlledBy(current, cluster)).To(BeTrue())
		}, timeout, interval).Should(Succeed())
		expectTestObjectsUnowned(namespace.Name, &appsv1.Deployment{}, "cloudprem-metastore-ro")

		By("creating the Worker next to the Observability Pipelines Worker manifest")
		Eventually(func(g Gomega) {
			worker := &datadoghqv1alpha1.DatadogObservabilityPipelinesWorker{}
			g.Expect(k8sClient.Get(context.Background(), client.ObjectKey{Name: "cloudprem-pipeline-logs", Namespace: namespace.Name}, worker)).To(Succeed())
			g.Expect(metav1.IsControlledBy(worker, cluster)).To(BeTrue())
			g.Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(worker), &appsv1.StatefulSet{})).To(Succeed())
		}, timeout, interval).Should(Succeed())
		expectTestObjectsUnowned(namespace.Name, &appsv1.StatefulSet{}, "logs-observability-pipelines-worker")
	})

	It("compares the live objects with the objects after the migration, and writes nothing", func() {
		By("adding a sidecar to the indexer, which the cluster cannot express")
		indexer := &appsv1.StatefulSet{}
		Expect(k8sClient.Get(context.Background(), client.ObjectKey{Name: "cloudprem-indexer", Namespace: namespace.Name}, indexer)).To(Succeed())
		indexer.Spec.Template.Spec.Containers = append(indexer.Spec.Template.Spec.Containers, corev1.Container{Name: "sidecar", Image: "busybox"})
		Expect(k8sClient.Update(context.Background(), indexer, client.FieldOwner(helmFieldManager))).To(Succeed())

		diff, findings, err := m.Diff(context.Background(), cloudprem, pipelines, converted)

		Expect(err).NotTo(HaveOccurred())
		Expect(findings).To(BeEmpty())
		Expect(diff).To(ContainSubstring("--- StatefulSet/cloudprem-indexer (live)\n+++ StatefulSet/cloudprem-indexer (after the migration)"))
		Expect(diff).To(MatchRegexp(`(?m)^-\s+name: sidecar$`))
		By("not showing the variables that only move")
		Expect(diff).NotTo(MatchRegexp(`(?m)^[-+]\s+- name: QW_INGEST_DECOMMISSION_TIMEOUT$`))
		Expect(diff).To(ContainSubstring("--- StatefulSet/logs-observability-pipelines-worker (live)\n+++ StatefulSet/cloudprem-pipeline-logs (after the migration)"))
		By("keeping the API port of the Worker Service")
		Expect(diff).To(ContainSubstring("--- Service/logs-observability-pipelines-worker (live)\n+++ Service/cloudprem-pipeline-logs (after the migration)"))
		Expect(diff).NotTo(MatchRegexp(`(?m)^-\s+port: 8686$`))
		By("listing the objects that the cluster replaces")
		Expect(diff).To(ContainSubstring("# The cluster replaces these objects of the manifests; delete them after the migration:\n"))
		Expect(diff).To(ContainSubstring("#   StatefulSet/logs-observability-pipelines-worker\n"))

		By("writing nothing")
		Expect(apierrors.IsNotFound(k8sClient.Get(context.Background(), client.ObjectKey{Name: "cloudprem", Namespace: namespace.Name}, &datadoghqv1alpha1.DatadogBYOCCluster{}))).To(BeTrue())
		Expect(apierrors.IsNotFound(k8sClient.Get(context.Background(), client.ObjectKey{Name: "cloudprem-pipeline-logs", Namespace: namespace.Name}, &appsv1.StatefulSet{}))).To(BeTrue())
		expectTestObjectsUnowned(namespace.Name, &appsv1.StatefulSet{}, "cloudprem-indexer")
		Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(indexer), indexer)).To(Succeed())
		Expect(indexer.Spec.Template.Spec.Containers).To(ContainElement(HaveField("Name", "sidecar")))
	})

	It("reports an object of the manifest that does not exist as a blocker", func() {
		Expect(k8sClient.Delete(context.Background(), &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "cloudprem", Namespace: namespace.Name}})).To(Succeed())

		_, findings, err := m.Diff(context.Background(), cloudprem, pipelines, converted)

		Expect(err).NotTo(HaveOccurred())
		Expect(testFindingStrings(findings)).To(ContainElement("Blocker: ConfigMap/cloudprem of manifest cloudprem does not exist in namespace " + namespace.Name + "; check the context and the namespace of the command, and that the manifest is the one that was applied"))
	})

	It("reports a resource that the API server refuses as a blocker", func() {
		edited := converted.DeepCopy()
		edited.Spec.Components.Indexer.Storage.VolumeClaimTemplate.Spec.Resources.Requests[corev1.ResourceStorage] = resource.MustParse("200Gi")

		_, findings, err := m.Diff(context.Background(), cloudprem, pipelines, edited)

		Expect(err).NotTo(HaveOccurred())
		Expect(testFindingStrings(findings)).To(ContainElement(And(
			HavePrefix("Blocker: StatefulSet/cloudprem-indexer: the API server refuses the resource of the cluster:"),
			ContainSubstring("Forbidden: updates to statefulset spec"),
		)))
	})

	It("writes nothing when a resource of the manifest has another controller", func() {
		configMap := &corev1.ConfigMap{}
		Expect(k8sClient.Get(context.Background(), client.ObjectKey{Name: "cloudprem", Namespace: namespace.Name}, configMap)).To(Succeed())
		configMap.OwnerReferences = []metav1.OwnerReference{{APIVersion: "v1", Kind: "ConfigMap", Name: "other", UID: "other-uid", Controller: ptr.To(true)}}
		Expect(k8sClient.Update(context.Background(), configMap)).To(Succeed())

		applyCluster()

		err := adopt()
		Expect(err).To(MatchError(ContainSubstring("ConfigMap/cloudprem is controlled by another owner or is being deleted")))

		expectTestObjectsUnowned(namespace.Name, &appsv1.StatefulSet{}, "cloudprem-indexer")
	})

	It("writes nothing when an object of the manifest does not exist", func() {
		Expect(k8sClient.Delete(context.Background(), &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "cloudprem", Namespace: namespace.Name}})).To(Succeed())

		applyCluster()

		err := adopt()
		Expect(err).To(MatchError(ContainSubstring("ConfigMap/cloudprem of manifest cloudprem does not exist in namespace " + namespace.Name)))

		expectTestObjectsUnowned(namespace.Name, &appsv1.StatefulSet{}, "cloudprem-indexer")
	})

	It("writes nothing when the adoption is not confirmed", func() {
		applyCluster()
		m.Confirm = func() error { return errors.New("not confirmed") }

		Expect(adopt()).To(MatchError("not confirmed"))

		expectTestObjectsUnowned(namespace.Name, &appsv1.StatefulSet{}, "cloudprem-indexer")
	})

	It("writes nothing when the user cannot patch the CloudPrem resources", func() {
		applyCluster()
		reader := readerMigration(namespace.Name, m.Cluster)

		err := reader.Adopt(context.Background(), mustLivePlan(reader, cloudprem, pipelines))

		Expect(err).To(MatchError(ContainSubstring("cannot give the resources of manifest cloudprem to the DatadogBYOCCluster, so nothing was written")))
		Expect(err).To(MatchError(ContainSubstring("StatefulSet/cloudprem-indexer: no permission to patch statefulsets.apps in namespace " + namespace.Name)))
		Expect(err).To(MatchError(ContainSubstring("ConfigMap/cloudprem: no permission to patch configmaps in namespace " + namespace.Name)))
		expectTestObjectsUnowned(namespace.Name, &appsv1.StatefulSet{}, "cloudprem-indexer")
		expectTestObjectsUnowned(namespace.Name, &corev1.ConfigMap{}, "cloudprem")
	})

	It("writes nothing when the user cannot remove the cluster from the owners of the CloudPrem resources", func() {
		applyCluster()
		Expect(adopt()).To(Succeed())
		Eventually(func(g Gomega) { markTestWorkloadsReady(g, namespace.Name) }, timeout, interval).Should(Succeed())
		reader := readerMigration(namespace.Name, m.Cluster)

		err := reader.Abort(context.Background(), cloudprem, pipelines)

		Expect(err).To(MatchError(ContainSubstring("cannot remove DatadogBYOCCluster " + namespace.Name + "/cloudprem from the owners of the objects of manifest cloudprem, so nothing was written")))
		Expect(err).To(MatchError(ContainSubstring("StatefulSet/cloudprem-indexer: no permission to patch statefulsets.apps in namespace " + namespace.Name)))
		cluster := &datadoghqv1alpha1.DatadogBYOCCluster{}
		Expect(k8sClient.Get(context.Background(), m.Cluster, cluster)).To(Succeed())
		indexer := &appsv1.StatefulSet{}
		Expect(k8sClient.Get(context.Background(), client.ObjectKey{Name: "cloudprem-indexer", Namespace: namespace.Name}, indexer)).To(Succeed())
		Expect(metav1.IsControlledBy(indexer, cluster)).To(BeTrue())
	})

	It("runs adopt again", func() {
		applyCluster()
		Expect(adopt()).To(Succeed())

		Expect(adopt()).To(Succeed())
	})

	It("writes nothing when the cluster does not exist", func() {
		err := adopt()

		Expect(err).To(MatchError(ContainSubstring("DatadogBYOCCluster " + namespace.Name + "/cloudprem does not exist; apply the cluster that convert printed")))
		expectTestObjectsUnowned(namespace.Name, &appsv1.StatefulSet{}, "cloudprem-indexer")
	})

	It("compares the live objects with the resources of the live cluster once it is applied", func() {
		applied := converted.DeepCopy()
		applied.Spec.Components.ControlPlane.Replicas = ptr.To[int32](5)
		Expect(k8sClient.Create(context.Background(), applied)).To(Succeed())

		diff, _, err := m.Diff(context.Background(), cloudprem, pipelines, nil)

		Expect(err).NotTo(HaveOccurred())
		Expect(diff).To(ContainSubstring("--- Deployment/cloudprem-control-plane (live)\n+++ Deployment/cloudprem-control-plane (after the migration)"))
		Expect(diff).To(MatchRegexp(`(?m)^\+\s+replicas: 5$`))
	})

	It("fails to compare a file once the cluster is applied", func() {
		applyCluster()

		_, _, err := m.Diff(context.Background(), cloudprem, pipelines, converted)

		Expect(err).To(MatchError(ContainSubstring("exists, and diff compares the objects with its resources; omit --from-file")))
	})

	It("fails to compare without a file before the cluster is applied", func() {
		_, _, err := m.Diff(context.Background(), cloudprem, pipelines, nil)

		Expect(err).To(MatchError(ContainSubstring("does not exist; pass the cluster that convert printed with --from-file")))
	})

	It("reverts a migration once the StatefulSets of the manifests run their Pods again", func() {
		applyCluster()
		Expect(adopt()).To(Succeed())
		expectClusterAvailable(namespace.Name, client.ObjectKeyFromObject(converted))
		manifestWorker := &appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Name: "logs-observability-pipelines-worker", Namespace: namespace.Name}}
		By("scaling the StatefulSet of the manifest to zero, as the user does once the Worker takes the traffic")
		scaleTestWorkload(manifestWorker, 0)
		abort := func(ctx context.Context) error { return m.Abort(ctx, cloudprem, pipelines) }

		By("writing nothing while the StatefulSet of the manifest is scaled to zero")
		Expect(abort(context.Background())).To(MatchError(ContainSubstring("StatefulSet/logs-observability-pipelines-worker of manifest logs is scaled to zero")))
		cluster := &datadoghqv1alpha1.DatadogBYOCCluster{}
		Expect(k8sClient.Get(context.Background(), client.ObjectKey{Name: "cloudprem", Namespace: namespace.Name}, cluster)).To(Succeed())
		indexer := &appsv1.StatefulSet{}
		Expect(k8sClient.Get(context.Background(), client.ObjectKey{Name: "cloudprem-indexer", Namespace: namespace.Name}, indexer)).To(Succeed())
		Expect(metav1.IsControlledBy(indexer, cluster)).To(BeTrue())

		By("scaling the StatefulSet of the manifest back, as the user does before abort")
		scaleTestWorkload(manifestWorker, 2)
		Eventually(func(g Gomega) { markTestWorkloadsReady(g, namespace.Name) }, timeout, interval).Should(Succeed())
		Expect(abort(context.Background())).To(Succeed())

		By("removing the cluster from the owners of the CloudPrem resources, and leaving its deletion to the user")
		expectTestObjectsUnowned(namespace.Name, &appsv1.StatefulSet{}, "cloudprem-indexer")
		expectTestObjectsUnowned(namespace.Name, &corev1.Service{}, "cloudprem-indexer")
		expectTestObjectsUnowned(namespace.Name, &corev1.ConfigMap{}, "cloudprem")
		Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(cluster), cluster)).To(Succeed())
		Expect(cluster.DeletionTimestamp.IsZero()).To(BeTrue())
	})

	It("removes a cluster being deleted from the owners of the CloudPrem resources", func() {
		applyCluster()
		Expect(adopt()).To(Succeed())
		cluster := &datadoghqv1alpha1.DatadogBYOCCluster{}
		Expect(k8sClient.Get(context.Background(), client.ObjectKey{Name: "cloudprem", Namespace: namespace.Name}, cluster)).To(Succeed())
		By("deleting the cluster, which a test finalizer keeps until the end of the test")
		Eventually(func() error {
			current := &datadoghqv1alpha1.DatadogBYOCCluster{}
			if err := k8sClient.Get(context.Background(), client.ObjectKeyFromObject(cluster), current); err != nil {
				return err
			}
			current.Finalizers = append(current.Finalizers, testMigrationFinalizer)
			return k8sClient.Update(context.Background(), current)
		}, timeout, interval).Should(Succeed())
		DeferCleanup(removeBYOCTestFinalizers, cluster, testMigrationFinalizer)
		Expect(k8sClient.Delete(context.Background(), cluster)).To(Succeed())

		Expect(m.Abort(context.Background(), cloudprem, pipelines)).To(Succeed())

		expectTestObjectsUnowned(namespace.Name, &corev1.Service{}, "cloudprem-indexer")
		expectTestObjectsUnowned(namespace.Name, &corev1.ConfigMap{}, "cloudprem")
	})
})

// testMigrationFinalizer keeps a deleted cluster until the test removes it.
const testMigrationFinalizer = "test.datadoghq.com/migration"

// installTestManifest creates the objects of a manifest rendered with helm template, as the Helm 3 CLI does.
func installTestManifest(namespace, name, file string) converter.Manifest {
	manifest, err := os.ReadFile(filepath.Join("..", "..", "cmd", "kubectl-byoc-migrate", "byocmigrate", "converter", "testdata", file))
	Expect(err).NotTo(HaveOccurred())
	installed, err := converter.NewManifest(name, namespace, string(manifest))
	Expect(err).NotTo(HaveOccurred())
	for _, object := range installed.Objects {
		// The fixtures were rendered for another namespace.
		object.SetNamespace(namespace)
		Expect(k8sClient.Create(context.Background(), object.DeepCopyObject().(client.Object), client.FieldOwner(helmFieldManager))).To(Succeed())
	}
	return installed
}

// readerMigration returns a migration whose user may read the objects of the namespace, but not write them.
func readerMigration(namespace string, cluster client.ObjectKey) *migration.Migration {
	name := "reader-" + namespace
	user, err := testEnv.AddUser(envtest.User{Name: name}, nil)
	Expect(err).NotTo(HaveOccurred())
	role := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{Name: "reader", Namespace: namespace},
		Rules:      []rbacv1.PolicyRule{{APIGroups: []string{"*"}, Resources: []string{"*"}, Verbs: []string{"get"}}},
	}
	Expect(k8sClient.Create(context.Background(), role)).To(Succeed())
	binding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "reader", Namespace: namespace},
		RoleRef:    rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "Role", Name: role.Name},
		Subjects:   []rbacv1.Subject{{APIGroup: rbacv1.GroupName, Kind: rbacv1.UserKind, Name: name}},
	}
	Expect(k8sClient.Create(context.Background(), binding)).To(Succeed())
	reader, err := client.New(user.Config(), client.Options{Scheme: k8sClient.Scheme()})
	Expect(err).NotTo(HaveOccurred())
	By("waiting for the API server to authorize the reads of the user")
	Eventually(func() error {
		return reader.Get(context.Background(), cluster, &datadoghqv1alpha1.DatadogBYOCCluster{})
	}, timeout, interval).Should(Succeed())
	return &migration.Migration{Client: reader, Cluster: cluster, Out: GinkgoWriter}
}

// mustLivePlan builds the plan of the live cluster, as the command does before adopt.
func mustLivePlan(m *migration.Migration, cloudprem converter.Manifest, pipelines []converter.PipelineManifest) *converter.Plan {
	plan, err := m.LivePlan(context.Background(), cloudprem, pipelines)
	Expect(err).NotTo(HaveOccurred())
	return plan
}

// scaleTestWorkload sets the replicas of a workload through its scale subresource, as an HPA does.
func scaleTestWorkload(workload client.Object, replicas int32) {
	Eventually(func() error {
		scale := &autoscalingv1.Scale{}
		if err := k8sClient.SubResource("scale").Get(context.Background(), workload, scale); err != nil {
			return err
		}
		scale.Spec.Replicas = replicas
		return k8sClient.SubResource("scale").Update(context.Background(), workload, client.WithSubResourceBody(scale), client.FieldOwner("hpa"))
	}, timeout, interval).Should(Succeed())
}

// expectClusterAvailable reports the workloads of the namespace as ready until the cluster is available, because
// envtest does not run the Deployment and StatefulSet controllers.
func expectClusterAvailable(namespace string, key client.ObjectKey) {
	Eventually(func(g Gomega) {
		markTestWorkloadsReady(g, namespace)
		cluster := &datadoghqv1alpha1.DatadogBYOCCluster{}
		g.Expect(k8sClient.Get(context.Background(), key, cluster)).To(Succeed())
		g.Expect(apimeta.IsStatusConditionTrue(cluster.Status.Conditions, "Available")).To(BeTrue())
	}, migrationTimeout, interval).Should(Succeed())
}

func markTestWorkloadsReady(g Gomega, namespace string) {
	deployments := &appsv1.DeploymentList{}
	g.Expect(k8sClient.List(context.Background(), deployments, client.InNamespace(namespace))).To(Succeed())
	for i := range deployments.Items {
		deployment := &deployments.Items[i]
		replicas := ptr.Deref(deployment.Spec.Replicas, 1)
		deployment.Status = appsv1.DeploymentStatus{ObservedGeneration: deployment.Generation, Replicas: replicas, ReadyReplicas: replicas, AvailableReplicas: replicas, UpdatedReplicas: replicas}
		g.Expect(client.IgnoreNotFound(k8sClient.Status().Update(context.Background(), deployment))).To(Succeed())
	}
	statefulSets := &appsv1.StatefulSetList{}
	g.Expect(k8sClient.List(context.Background(), statefulSets, client.InNamespace(namespace))).To(Succeed())
	for i := range statefulSets.Items {
		statefulSet := &statefulSets.Items[i]
		replicas := ptr.Deref(statefulSet.Spec.Replicas, 1)
		statefulSet.Status = appsv1.StatefulSetStatus{ObservedGeneration: statefulSet.Generation, Replicas: replicas, ReadyReplicas: replicas, AvailableReplicas: replicas, UpdatedReplicas: replicas}
		g.Expect(client.IgnoreNotFound(k8sClient.Status().Update(context.Background(), statefulSet))).To(Succeed())
	}
}

// expectTestObjectsUnowned checks that an object exists and has no owner.
func expectTestObjectsUnowned(namespace string, object client.Object, name string) {
	Expect(k8sClient.Get(context.Background(), client.ObjectKey{Name: name, Namespace: namespace}, object)).To(Succeed())
	Expect(object.GetOwnerReferences()).To(BeEmpty(), "%T %s", object, name)
}

// testManagers returns the field managers of an object, with the subresource of their entry, if any.
func testManagers(object client.Object) []string {
	var managers []string
	for _, entry := range object.GetManagedFields() {
		manager := entry.Manager
		if entry.Subresource != "" {
			manager += "/" + entry.Subresource
		}
		managers = append(managers, manager)
	}
	return managers
}

// testFindingStrings returns the findings as the commands print them.
func testFindingStrings(findings []converter.Finding) []string {
	printed := make([]string, 0, len(findings))
	for _, finding := range findings {
		printed = append(printed, finding.String())
	}
	return printed
}
