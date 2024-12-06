// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package merger

import (
	"testing"

	"github.com/DataDog/datadog-operator/api/crds/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/store"
	cilium "github.com/DataDog/datadog-operator/pkg/cilium/v1"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestCiliumPolicyManager_AddCiliumPolicy(t *testing.T) {
	ns := "bar"
	name1 := "foo"
	name2 := "foo2"

	podSelector := metav1.LabelSelector{
		MatchLabels: map[string]string{
			kubernetes.AppKubernetesInstanceLabelKey: "policy",
			kubernetes.AppKubernetesPartOfLabelKey:   "partof",
		},
	}

	policySpec1 := []cilium.NetworkPolicySpec{
		{
			Description:      "Egress 1",
			EndpointSelector: podSelector,
			Egress: []cilium.EgressRule{
				{
					ToEndpoints: []metav1.LabelSelector{
						{
							MatchExpressions: []metav1.LabelSelectorRequirement{
								{
									Key:      "k8s:io.kubernetes.pod.namespace",
									Operator: "Exists",
								},
							},
						},
					},
				},
			},
		},
	}

	existingPolicy := cilium.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns,
			Name:      name2,
		},
		Specs: []cilium.NetworkPolicySpec{
			{
				Description:      "Existing egress",
				EndpointSelector: podSelector,
				Egress: []cilium.EgressRule{
					{
						ToPorts: []cilium.PortRule{
							{
								Ports: []cilium.PortProtocol{
									{
										Port:     "1001",
										Protocol: cilium.ProtocolTCP,
									},
								},
							},
						},
					},
				},
			},
		},
	}
	unstructuredPolicy := &unstructured.Unstructured{}
	var err error
	unstructuredPolicy.Object, err = runtime.DefaultUnstructuredConverter.ToUnstructured(&existingPolicy)
	if err != nil {
		t.Errorf("unable to convert cilium network policy %s/%s to unstructured object: %s", name2, ns, err)
	}
	unstructuredPolicy.SetGroupVersionKind(cilium.GroupVersionCiliumNetworkPolicyKind())

	testScheme := runtime.NewScheme()
	testScheme.AddKnownTypes(v2alpha1.GroupVersion, &v2alpha1.DatadogAgent{})
	storeOptions := &store.StoreOptions{
		Scheme: testScheme,
	}

	owner := &v2alpha1.DatadogAgent{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns,
			Name:      name1,
		},
	}

	type args struct {
		namespace  string
		name       string
		policySpec []cilium.NetworkPolicySpec
	}
	tests := []struct {
		name         string
		store        *store.Store
		args         args
		wantErr      bool
		validateFunc func(*testing.T, *store.Store)
	}{
		{
			name:  "empty store",
			store: store.NewStore(owner, storeOptions),
			args: args{
				namespace:  ns,
				name:       name1,
				policySpec: policySpec1,
			},
			wantErr: false,
			validateFunc: func(t *testing.T, store *store.Store) {
				if _, found := store.Get(kubernetes.CiliumNetworkPoliciesKind, ns, name1); !found {
					t.Errorf("missing CiliumPolicy %s/%s", ns, name1)
				}
			},
		},
		{
			name:  "another CiliumPolicy already exists",
			store: store.NewStore(owner, storeOptions).AddOrUpdateStore(kubernetes.CiliumNetworkPoliciesKind, unstructuredPolicy),
			args: args{
				namespace:  ns,
				name:       name1,
				policySpec: policySpec1,
			},
			wantErr: false,
			validateFunc: func(t *testing.T, store *store.Store) {
				if _, found := store.Get(kubernetes.CiliumNetworkPoliciesKind, ns, name1); !found {
					t.Errorf("missing CiliumPolicy %s/%s", ns, name1)
				}
			},
		},
		{
			name:  "update existing CiliumPolicy",
			store: store.NewStore(owner, storeOptions).AddOrUpdateStore(kubernetes.CiliumNetworkPoliciesKind, unstructuredPolicy),
			args: args{
				namespace:  ns,
				name:       name2,
				policySpec: policySpec1,
			},
			wantErr: false,
			validateFunc: func(t *testing.T, store *store.Store) {
				obj, found := store.Get(kubernetes.CiliumNetworkPoliciesKind, ns, name2)
				if !found {
					t.Errorf("missing CiliumPolicy %s/%s", ns, name2)
				}
				policy, ok := obj.(*unstructured.Unstructured)
				uPolicy := policy.UnstructuredContent()
				var typedPolicy cilium.NetworkPolicy
				err := runtime.DefaultUnstructuredConverter.FromUnstructured(uPolicy, &typedPolicy)
				if err != nil {
					t.Errorf("unable to convert unstructured object %s/%s to cilium network policy: %s", ns, name2, err)
				}
				if !ok || len(typedPolicy.Specs) != 2 {
					t.Errorf("missing Egress in CiliumPolicy %s/%s", ns, name2)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &ciliumPolicyManagerImpl{
				store: tt.store,
			}
			if err := m.AddCiliumPolicy(tt.args.name, tt.args.namespace, tt.args.policySpec); (err != nil) != tt.wantErr {
				t.Errorf("CiliumPolicyManager.AddCiliumPolicy() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.validateFunc != nil {
				tt.validateFunc(t, tt.store)
			}
		})
	}
}
