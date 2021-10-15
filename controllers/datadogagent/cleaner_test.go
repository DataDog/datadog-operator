// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagent

import (
	"reflect"
	"testing"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
	test "github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1/test"
	kversion "k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func Test_cleanupOldClusterRBAC(t *testing.T) {
	t.Helper()
	logf.SetLogger(zap.New(zap.UseDevMode(true)))
	logger := logf.Log.WithName(t.Name())

	s := scheme.Scheme

	ddaName := "foo"
	ddaNamespace := "bar"
	ddaDefaultOptions := &test.NewDatadogAgentOptions{
		ClusterAgentEnabled:        true,
		ClusterChecksEnabled:       true,
		ClusterChecksRunnerEnabled: true,
		KubeStateMetricsCore: &datadoghqv1alpha1.KubeStateMetricsCore{
			Enabled:      datadoghqv1alpha1.NewBoolPointer(true),
			ClusterCheck: datadoghqv1alpha1.NewBoolPointer(true),
		},
	}
	defaultDDA := test.NewDefaultedDatadogAgent(ddaNamespace, ddaName, ddaDefaultOptions)
	agentVersion := getAgentVersion(defaultDDA)
	serviceAccountName := getClusterChecksRunnerServiceAccount(defaultDDA)
	roleName := "cluster-agent"
	rbacResourcesName := getClusterChecksRunnerRbacResourcesName(defaultDDA)
	clusterRoleBindingInfo := roleBindingInfo{
		name:               rbacResourcesName,
		roleName:           roleName,
		serviceAccountName: serviceAccountName,
	}

	defaultClusterRoleBinding := buildClusterRoleBinding(defaultDDA, clusterRoleBindingInfo, agentVersion)

	defaultClusterAgentClusterRoleRBAC := buildClusterAgentClusterRole(defaultDDA, rbacResourcesName, agentVersion)

	ksmClusterRoleRBAC := buildKubeStateMetricsCoreRBAC(defaultDDA, getKubeStateMetricsRBACResourceName(defaultDDA, checkRunnersSuffix), agentVersion)

	oldClusterAgentClusterRoleRBAC := defaultClusterAgentClusterRoleRBAC.DeepCopy()
	oldClusterAgentClusterRoleRBAC.Name = oldClusterAgentClusterRoleRBAC.Name + "-old"

	type args struct {
		k8sClient client.Client
		version   *kversion.Info
		dda       *datadoghqv1alpha1.DatadogAgent
	}
	tests := []struct {
		name    string
		args    args
		want    []client.Object
		wantErr bool
	}{
		{
			name: "nothing to clean",
			args: args{
				k8sClient: fake.NewClientBuilder().WithScheme(s).Build(),
				version:   &kversion.Info{},
				dda:       defaultDDA,
			},
			wantErr: false,
			want:    nil,
		},
		{
			name: "nothing to clean, existing RBAC",
			args: args{
				k8sClient: fake.NewClientBuilder().WithScheme(s).WithObjects(defaultClusterRoleBinding, defaultClusterAgentClusterRoleRBAC, ksmClusterRoleRBAC).Build(),
				version:   &kversion.Info{},
				dda:       defaultDDA,
			},
			wantErr: false,
			want:    nil,
		},
		{
			name: "delete 1 resources, existing RBAC",
			args: args{
				k8sClient: fake.NewClientBuilder().WithScheme(s).WithObjects(defaultClusterRoleBinding, defaultClusterAgentClusterRoleRBAC, ksmClusterRoleRBAC, oldClusterAgentClusterRoleRBAC).Build(),
				version:   &kversion.Info{},
				dda:       defaultDDA,
			},
			wantErr: false,
			want:    []client.Object{oldClusterAgentClusterRoleRBAC},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := cleanupOldClusterRBACs(logger.WithName(tt.name), tt.args.k8sClient, tt.args.version, tt.args.dda)
			if (err != nil) != tt.wantErr {
				t.Errorf("cleanupOldClusterRBAC() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("cleanupOldClusterRBAC() = %v, want %v", got, tt.want)
			}
		})
	}
}
