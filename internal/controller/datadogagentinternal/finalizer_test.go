package datadogagentinternal

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"k8s.io/utils/ptr"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	datadoghqv2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/component"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/component/agent"
	agenttestutils "github.com/DataDog/datadog-operator/internal/controller/datadogagent/testutils"
	"github.com/DataDog/datadog-operator/internal/controller/testutils"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/kubernetes/rbac"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func Test_handleFinalizer(t *testing.T) {
	// This is not an exhaustive test. The finalizer should remove all the
	// kubernetes resources associated with the DatadogAgentInternal being removed, but
	// to simplify a bit, this test doesn't check all the resources, it just
	// checks a few ones (cluster roles, cluster role bindings, profile labels).

	now := metav1.Now()
	operatorStoreLabels := map[string]string{
		"operator.datadoghq.com/managed-by-store": "true",
		"app.kubernetes.io/part-of":               "foo-bar",
		"app.kubernetes.io/managed-by":            "datadog-operator",
	}

	ddai := &datadoghqv1alpha1.DatadogAgentInternal{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:  "foo",
			Name:       "bar",
			Finalizers: []string{constants.DatadogAgentInternalFinalizer},
		},
		Spec: datadoghqv2alpha1.DatadogAgentSpec{
			Global: &datadoghqv2alpha1.GlobalConfig{
				Credentials: &datadoghqv2alpha1.DatadogCredentials{
					APIKey: ptr.To("apiKey"),
					AppKey: ptr.To("appKey"),
				},
			},
		},
	}
	ddai.DeletionTimestamp = &now // Mark for deletion

	initialKubeObjects := []client.Object{ddai}

	// These are some cluster roles that we know that the reconciler creates by
	// default
	existingClusterRoles := []*rbacv1.ClusterRole{
		{
			TypeMeta: metav1.TypeMeta{
				Kind:       rbac.ClusterRoleKind,
				APIVersion: rbacv1.SchemeGroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:   agent.GetAgentRoleName(ddai),
				Labels: operatorStoreLabels,
			},
		},
		{
			TypeMeta: metav1.TypeMeta{
				Kind:       rbac.ClusterRoleKind,
				APIVersion: rbacv1.SchemeGroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:   component.GetClusterAgentName(ddai),
				Labels: operatorStoreLabels,
			},
		},
	}

	// These are some cluster role bindings that we know that the reconciler
	// creates by default
	existingClusterRoleBindings := []*rbacv1.ClusterRoleBinding{
		{
			TypeMeta: metav1.TypeMeta{
				Kind:       rbac.ClusterRoleBindingKind,
				APIVersion: rbacv1.SchemeGroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:   agent.GetAgentRoleName(ddai), // Same name as the cluster role
				Labels: operatorStoreLabels,
			},
		},
		{
			TypeMeta: metav1.TypeMeta{
				Kind:       rbac.ClusterRoleBindingKind,
				APIVersion: rbacv1.SchemeGroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:   component.GetClusterAgentName(ddai),
				Labels: operatorStoreLabels,
			},
		},
	}

	nodes := []*corev1.Node{
		testutils.NewNode("node-1", nil),
		testutils.NewNode("node-2", map[string]string{constants.ProfileLabelKey: "true"}), // The label should be deleted
	}

	for _, clusterRole := range existingClusterRoles {
		initialKubeObjects = append(initialKubeObjects, clusterRole)
	}

	for _, clusterRoleBinding := range existingClusterRoleBindings {
		initialKubeObjects = append(initialKubeObjects, clusterRoleBinding)
	}

	for _, node := range nodes {
		initialKubeObjects = append(initialKubeObjects, node)
	}

	reconciler := reconcilerForFinalizerTest(initialKubeObjects)

	err := reconciler.finalizeDDAI(context.Background(), ddai)
	assert.NoError(t, err)

	// Check that the cluster roles associated with the Datadog Agent have been deleted
	for _, clusterRole := range existingClusterRoles {
		err = reconciler.client.Get(context.TODO(), types.NamespacedName{Name: clusterRole.Name}, &rbacv1.ClusterRole{})
		assert.Error(t, err, fmt.Sprintf("ClusterRole %s not deleted", clusterRole.Name))
		if err != nil {
			assert.True(t, apierrors.IsNotFound(err), fmt.Sprintf("Unexpected error %s", err))
		}
	}

	// Check that the cluster role bindings associated with the Datadog Agent have been deleted
	for _, clusterRoleBinding := range existingClusterRoleBindings {
		err = reconciler.client.Get(context.TODO(), types.NamespacedName{Name: clusterRoleBinding.Name}, &rbacv1.ClusterRoleBinding{})
		assert.Error(t, err, fmt.Sprintf("ClusterRoleBinding %s not deleted", clusterRoleBinding.Name))
		if err != nil {
			assert.True(t, apierrors.IsNotFound(err), fmt.Sprintf("Unexpected error %s", err))
		}
	}

	// Check that the nodes don't have the profile label anymore
	for _, node := range nodes {
		currentNode := &corev1.Node{}
		err = reconciler.client.Get(context.TODO(), types.NamespacedName{Name: node.Name}, currentNode)
		assert.NoError(t, err)
		assert.NotContains(t, currentNode.Labels, constants.ProfileLabelKey)
	}
}

func TestPreparedRolloutFinalizerDeletesWorkloadsBeforeReleasingNodeLabels(t *testing.T) {
	ctx := context.Background()
	now := metav1.Now()
	ddai := &datadoghqv1alpha1.DatadogAgentInternal{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:         "foo",
			Name:              "bar",
			UID:               "ddai-uid",
			DeletionTimestamp: &now,
			Finalizers:        []string{constants.DatadogAgentInternalFinalizer},
		},
	}
	rolloutKey := preparedRolloutNodeLabelKey(ddai)
	candidateKey := preparedRolloutCandidateAnnotationKey(rolloutKey)
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{
		Name: "node-1",
		Labels: map[string]string{
			rolloutKey:                rolloutSlotGreen,
			constants.ProfileLabelKey: "profile-a",
		},
		Annotations: map[string]string{candidateKey: "candidate-uid"},
	}}
	owner := metav1.OwnerReference{
		APIVersion: datadoghqv1alpha1.GroupVersion.String(),
		Kind:       "DatadogAgentInternal",
		Name:       ddai.Name,
		UID:        ddai.UID,
		Controller: ptr.To(true),
	}
	preparedDaemonSet := func(name string) *appsv1.DaemonSet {
		return &appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{
			Namespace:       ddai.Namespace,
			Name:            name,
			OwnerReferences: []metav1.OwnerReference{owner},
		}, Spec: appsv1.DaemonSetSpec{Template: corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{preparedRolloutModeAnnotation: preparedBlueGreenMode},
		}}}}
	}
	blue := preparedDaemonSet("bar-agent")
	green := preparedDaemonSet("bar-agent-green")
	delete(green.Spec.Template.Annotations, preparedRolloutModeAnnotation)
	green.Annotations = map[string]string{preparedRolloutRevisionAnnotation: "persisted-revision"}
	reconciler := reconcilerForFinalizerTest([]client.Object{
		ddai,
		node,
		blue,
		green,
	})

	// The first pass initiates foreground deletion but deliberately retains
	// rollout state while the workloads are still API-observed.
	err := reconciler.finalizeDDAI(ctx, ddai)
	assert.ErrorContains(t, err, "waiting for prepared blue/green Agent workloads")
	currentNode := &corev1.Node{}
	assert.NoError(t, reconciler.client.Get(ctx, types.NamespacedName{Name: node.Name}, currentNode))
	assert.Equal(t, rolloutSlotGreen, currentNode.Labels[rolloutKey])
	assert.Equal(t, "profile-a", currentNode.Labels[constants.ProfileLabelKey])
	assert.Equal(t, "candidate-uid", currentNode.Annotations[candidateKey])

	// Once both controllers are gone, a retry can safely release node state and
	// let Kubernetes remove the parent object.
	assert.NoError(t, reconciler.finalizeDDAI(ctx, ddai))
	assert.NoError(t, reconciler.client.Get(ctx, types.NamespacedName{Name: node.Name}, currentNode))
	assert.NotContains(t, currentNode.Labels, rolloutKey)
	assert.NotContains(t, currentNode.Labels, constants.ProfileLabelKey)
	assert.NotContains(t, currentNode.Annotations, candidateKey)
}

func TestInternalReconcileV2TreatsPreparedWorkloadTerminationAsControllerState(t *testing.T) {
	ctx := context.Background()
	now := metav1.Now()
	ddai := &datadoghqv1alpha1.DatadogAgentInternal{ObjectMeta: metav1.ObjectMeta{
		Namespace:         "default",
		Name:              "agent",
		UID:               "agent-uid",
		DeletionTimestamp: &now,
		Finalizers:        []string{constants.DatadogAgentInternalFinalizer},
	}}
	rolloutKey := preparedRolloutNodeLabelKey(ddai)
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node", Labels: map[string]string{rolloutKey: rolloutSlotBlue}}}
	controller := true
	ds := &appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{
		Name:      "agent",
		Namespace: ddai.Namespace,
		OwnerReferences: []metav1.OwnerReference{{
			APIVersion: datadoghqv1alpha1.GroupVersion.String(), Kind: "DatadogAgentInternal", Name: ddai.Name, UID: ddai.UID, Controller: &controller,
		}},
	}, Spec: appsv1.DaemonSetSpec{Template: corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{
		Annotations: map[string]string{preparedRolloutModeAnnotation: preparedBlueGreenMode},
	}}}}
	r := reconcilerForFinalizerTest([]client.Object{ddai, node, ds})

	result, err := r.internalReconcileV2(ctx, ddai)
	require.NoError(t, err)
	assert.Equal(t, defaultErrRequeuePeriod, result.RequeueAfter)
	assert.Contains(t, ddai.Finalizers, constants.DatadogAgentInternalFinalizer)
	currentNode := &corev1.Node{}
	require.NoError(t, r.client.Get(ctx, client.ObjectKeyFromObject(node), currentNode))
	assert.Equal(t, rolloutSlotBlue, currentNode.Labels[rolloutKey], "node ownership remains until the child workload disappears")
}

func TestPreparedRolloutFinalizerPropagatesClientFailures(t *testing.T) {
	ctx := context.Background()
	injected := errors.New("injected client failure")
	ddai := &datadoghqv1alpha1.DatadogAgentInternal{ObjectMeta: metav1.ObjectMeta{Name: "agent", Namespace: "default", UID: "agent-uid"}}

	base := reconcilerForFinalizerTest(nil)
	base.client = &finalizerFailureClient{Client: base.client, listErr: injected}
	_, err := base.preparedRolloutWorkloadsCleanup(ctx, ddai)
	require.ErrorIs(t, err, injected)
	require.ErrorIs(t, base.preparedRolloutLabelsCleanup(ctx, ddai), injected)

	key := preparedRolloutNodeLabelKey(ddai)
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node", Labels: map[string]string{key: rolloutSlotBlue}}}
	base = reconcilerForFinalizerTest([]client.Object{node})
	base.client = &finalizerFailureClient{Client: base.client, patchErr: injected}
	require.ErrorIs(t, base.preparedRolloutLabelsCleanup(ctx, ddai), injected)

	controller := true
	ds := &appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{
		Name: "agent", Namespace: ddai.Namespace,
		OwnerReferences: []metav1.OwnerReference{{APIVersion: datadoghqv1alpha1.GroupVersion.String(), Kind: "DatadogAgentInternal", Name: ddai.Name, UID: ddai.UID, Controller: &controller}},
	}, Spec: appsv1.DaemonSetSpec{Template: corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{preparedRolloutModeAnnotation: preparedBlueGreenMode}}}}}
	base = reconcilerForFinalizerTest([]client.Object{ds})
	base.client = &finalizerFailureClient{Client: base.client, deleteErr: injected}
	_, err = base.preparedRolloutWorkloadsCleanup(ctx, ddai)
	require.ErrorIs(t, err, injected)
}

type finalizerFailureClient struct {
	client.Client
	listErr   error
	patchErr  error
	deleteErr error
}

func (c *finalizerFailureClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	if c.listErr != nil {
		return c.listErr
	}
	return c.Client.List(ctx, list, opts...)
}

func (c *finalizerFailureClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	if c.patchErr != nil {
		return c.patchErr
	}
	return c.Client.Patch(ctx, obj, patch, opts...)
}

func (c *finalizerFailureClient) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	if c.deleteErr != nil {
		return c.deleteErr
	}
	return c.Client.Delete(ctx, obj, opts...)
}

func reconcilerForFinalizerTest(initialKubeObjects []client.Object) Reconciler {
	s := agenttestutils.TestScheme()

	fakeClient := fake.NewClientBuilder().WithObjects(initialKubeObjects...).WithScheme(s).Build()

	return Reconciler{
		client:     fakeClient,
		scheme:     s,
		recorder:   record.NewBroadcaster().NewRecorder(s, corev1.EventSource{}),
		forwarders: dummyManager{},
		options:    ReconcilerOptions{},
	}
}
