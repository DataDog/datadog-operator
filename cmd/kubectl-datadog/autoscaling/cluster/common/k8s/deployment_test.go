package k8s

import (
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

func TestFindFirstDeployment(t *testing.T) {
	abc := []runtime.Object{deployment("ns", "a"), deployment("ns", "b"), deployment("ns", "c")}

	for _, tc := range []struct {
		name      string
		objects   []runtime.Object
		predicate func(*appsv1.Deployment) bool
		wantName  string // empty means the expected result is nil
		wantCalls int
	}{
		{
			name:      "empty cluster",
			objects:   nil,
			predicate: func(*appsv1.Deployment) bool { return true },
			wantName:  "",
			wantCalls: 0,
		},
		{
			name:      "single deployment, no match",
			objects:   []runtime.Object{deployment("ns", "a")},
			predicate: func(*appsv1.Deployment) bool { return false },
			wantName:  "",
			wantCalls: 1,
		},
		{
			name:      "single deployment, match",
			objects:   []runtime.Object{deployment("ns", "a")},
			predicate: func(*appsv1.Deployment) bool { return true },
			wantName:  "a",
			wantCalls: 1,
		},
		{
			name:      "many deployments, no match",
			objects:   abc,
			predicate: func(*appsv1.Deployment) bool { return false },
			wantName:  "",
			wantCalls: 3,
		},
		{
			name:      "many deployments, first matches (short-circuits)",
			objects:   abc,
			predicate: func(*appsv1.Deployment) bool { return true },
			wantName:  "a",
			wantCalls: 1,
		},
		{
			name:      "many deployments, middle matches",
			objects:   abc,
			predicate: func(d *appsv1.Deployment) bool { return d.Name == "b" },
			wantName:  "b",
			wantCalls: 2,
		},
		{
			name:      "many deployments, last matches (full scan)",
			objects:   abc,
			predicate: func(d *appsv1.Deployment) bool { return d.Name == "c" },
			wantName:  "c",
			wantCalls: 3,
		},
		{
			name:      "multiple match, first encountered wins",
			objects:   abc,
			predicate: func(d *appsv1.Deployment) bool { return d.Name == "b" || d.Name == "c" },
			wantName:  "b",
			wantCalls: 2,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cli := fake.NewSimpleClientset(tc.objects...)
			calls := 0
			counting := func(d *appsv1.Deployment) bool {
				calls++
				return tc.predicate(d)
			}

			got, err := FindFirstDeployment(t.Context(), cli, counting)

			require.NoError(t, err)
			if tc.wantName == "" {
				assert.Nil(t, got)
			} else {
				require.NotNil(t, got)
				assert.Equal(t, tc.wantName, got.Name)
			}
			assert.Equal(t, tc.wantCalls, calls,
				"predicate must be called exactly until the first match (or to exhaustion when none matches)")
		})
	}
}

func TestFindFirstDeployment_PropagatesListError(t *testing.T) {
	cli := fake.NewSimpleClientset()
	listErr := errors.New("api down")
	cli.PrependReactor("list", "deployments", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, listErr
	})

	got, err := FindFirstDeployment(t.Context(), cli, func(*appsv1.Deployment) bool { return true })

	require.Error(t, err)
	assert.ErrorIs(t, err, listErr)
	assert.Nil(t, got)
}
