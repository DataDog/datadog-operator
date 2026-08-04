// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package remoteconfig

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestGetUpdaterTags(t *testing.T) {
	clusterUID := types.UID("test-cluster-uid")

	tests := []struct {
		name        string
		clusterName string
		objects     []client.Object
		want        []string
	}{
		{
			name:        "with cluster name and cluster uid",
			clusterName: "test-cluster",
			objects: []client.Object{
				&corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: "kube-system",
						UID:  clusterUID,
					},
				},
			},
			want: []string{
				"updater_type:datadog-operator",
				"cluster_name:test-cluster",
				"cluster_id:" + string(clusterUID),
			},
		},
		{
			name:        "without cluster uid",
			clusterName: "test-cluster",
			want: []string{
				"updater_type:datadog-operator",
				"cluster_name:test-cluster",
			},
		},
		{
			name: "without cluster name",
			objects: []client.Object{
				&corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: "kube-system",
						UID:  clusterUID,
					},
				},
			},
			want: []string{
				"updater_type:datadog-operator",
				"cluster_id:" + string(clusterUID),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			updater := &RemoteConfigUpdater{
				kubeClient: newFakeClient(t, tt.objects...),
				logger:     logr.Discard(),
				serviceConf: RcServiceConfiguration{
					clusterName: tt.clusterName,
				},
			}

			assert.Equal(t, tt.want, updater.getUpdaterTags(context.Background()))
		})
	}
}

func newFakeClient(t *testing.T, objects ...client.Object) client.Client {
	t.Helper()

	scheme := runtime.NewScheme()
	assert.NoError(t, corev1.AddToScheme(scheme))

	return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build()
}
