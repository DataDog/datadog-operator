// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package merger

import (
	"testing"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/store"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"

	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestNetworkPolicyManager_AddKubernetesNetworkPolicy(t *testing.T) {
	ns := "bar"
	name1 := "foo"
	name2 := "foo2"

	podSelector := metav1.LabelSelector{
		MatchLabels: map[string]string{
			kubernetes.AppKubernetesInstanceLabelKey: "policy",
			kubernetes.AppKubernetesPartOfLabelKey:   "partof",
		},
	}

	policyTypes := []netv1.PolicyType{
		netv1.PolicyTypeIngress,
		netv1.PolicyTypeEgress,
	}

	egress1 := []netv1.NetworkPolicyEgressRule{
		{
			Ports: []netv1.NetworkPolicyPort{
				{
					Port: &intstr.IntOrString{
						Type:   intstr.Int,
						IntVal: 443,
					},
				},
			},
		},
	}

	egress2 := []netv1.NetworkPolicyEgressRule{
		{
			Ports: []netv1.NetworkPolicyPort{
				{
					Port: &intstr.IntOrString{
						Type:   intstr.Int,
						IntVal: 5000,
					},
				},
			},
		},
	}

	ingress := []netv1.NetworkPolicyIngressRule{
		{
			Ports: []netv1.NetworkPolicyPort{
				{
					Port: &intstr.IntOrString{
						Type:   intstr.Int,
						IntVal: 5000,
					},
				},
			},
			From: []netv1.NetworkPolicyPeer{
				{
					PodSelector: &podSelector,
				},
			},
		},
	}

	existingPolicy := netv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns,
			Name:      name2,
		},
		Spec: netv1.NetworkPolicySpec{
			PodSelector: podSelector,
			Egress:      egress2,
		},
	}

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
		namespace   string
		name        string
		podSelector metav1.LabelSelector
		policyTypes []netv1.PolicyType
		ingress     []netv1.NetworkPolicyIngressRule
		egress      []netv1.NetworkPolicyEgressRule
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
				namespace:   ns,
				name:        name1,
				podSelector: podSelector,
				policyTypes: policyTypes,
				ingress:     ingress,
				egress:      egress1,
			},
			wantErr: false,
			validateFunc: func(t *testing.T, store *store.Store) {
				if _, found := store.Get(kubernetes.NetworkPoliciesKind, ns, name1); !found {
					t.Errorf("missing NetworkPolicy %s/%s", ns, name1)
				}
			},
		},
		{
			name:  "another NetworkPolicy already exists",
			store: store.NewStore(owner, storeOptions).AddOrUpdateStore(kubernetes.NetworkPoliciesKind, &existingPolicy),
			args: args{
				namespace:   ns,
				name:        name1,
				podSelector: podSelector,
				policyTypes: policyTypes,
				ingress:     ingress,
				egress:      egress1,
			},
			wantErr: false,
			validateFunc: func(t *testing.T, store *store.Store) {
				if _, found := store.Get(kubernetes.NetworkPoliciesKind, ns, name1); !found {
					t.Errorf("missing NetworkPolicy %s/%s", ns, name1)
				}
			},
		},
		{
			name:  "update existing NetworkPolicy",
			store: store.NewStore(owner, storeOptions).AddOrUpdateStore(kubernetes.NetworkPoliciesKind, &existingPolicy),
			args: args{
				namespace:   ns,
				name:        name2,
				podSelector: podSelector,
				policyTypes: nil,
				ingress:     ingress,
				egress:      egress1,
			},
			wantErr: false,
			validateFunc: func(t *testing.T, store *store.Store) {
				obj, found := store.Get(kubernetes.NetworkPoliciesKind, ns, name2)
				if !found {
					t.Errorf("missing NetworkPolicy %s/%s", ns, name2)
				}
				policy, ok := obj.(*netv1.NetworkPolicy)
				if !ok || len(policy.Spec.Egress) != 2 {
					t.Errorf("missing Egress in NetworkPolicy %s/%s", ns, name2)
				}
				if len(policy.Spec.Ingress) != 1 {
					t.Errorf("missing Ingress in NetworkPolicy %s/%s", ns, name2)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &networkPolicyManagerImpl{
				store: tt.store,
			}
			if err := m.AddKubernetesNetworkPolicy(tt.args.name, tt.args.namespace, tt.args.podSelector, tt.args.policyTypes, tt.args.ingress, tt.args.egress); (err != nil) != tt.wantErr {
				t.Errorf("NetworkPolicyManager.AddKubernetesNetworkPolicy() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.validateFunc != nil {
				tt.validateFunc(t, tt.store)
			}
		})
	}
}
