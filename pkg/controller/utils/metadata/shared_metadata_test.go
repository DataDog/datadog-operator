// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package metadata

import (
	testutils "github.com/DataDog/datadog-operator/internal/controller/datadogagent/testutils"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// newFakeClientWithKubeSystem creates a fake Kubernetes client with a kube-system namespace.
func newFakeClientWithKubeSystem(clusterUID string) client.Client {
	s := testutils.TestScheme()
	kubeSystem := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "kube-system",
			UID:  types.UID(clusterUID),
		},
	}
	return fake.NewClientBuilder().WithScheme(s).WithObjects(kubeSystem).Build()
}
