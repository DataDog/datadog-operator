package k8s

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

func deployment(namespace, name string) *appsv1.Deployment {
	return &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name}}
}

func TestFindFirstDeployment_NoMatch(t *testing.T) {
	cli := fake.NewSimpleClientset(deployment("ns", "a"), deployment("ns", "b"))

	got, err := FindFirstDeployment(context.Background(), cli, func(appsv1.Deployment) bool { return false })

	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestFindFirstDeployment_ReturnsFirstMatch(t *testing.T) {
	cli := fake.NewSimpleClientset(deployment("ns", "a"), deployment("ns", "b"), deployment("ns", "c"))

	got, err := FindFirstDeployment(context.Background(), cli, func(d appsv1.Deployment) bool {
		return d.Name == "b" || d.Name == "c"
	})

	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Contains(t, []string{"b", "c"}, got.Name)
}

func TestFindFirstDeployment_ShortCircuits(t *testing.T) {
	cli := fake.NewSimpleClientset(deployment("ns", "a"), deployment("ns", "b"), deployment("ns", "c"))

	calls := 0
	got, err := FindFirstDeployment(context.Background(), cli, func(d appsv1.Deployment) bool {
		calls++
		return true
	})

	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, 1, calls, "predicate must stop being called after the first match")
}

func TestFindFirstDeployment_PropagatesListError(t *testing.T) {
	cli := fake.NewSimpleClientset()
	listErr := errors.New("api down")
	cli.PrependReactor("list", "deployments", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, listErr
	})

	got, err := FindFirstDeployment(context.Background(), cli, func(appsv1.Deployment) bool { return true })

	require.Error(t, err)
	assert.ErrorIs(t, err, listErr)
	assert.Nil(t, got)
}
