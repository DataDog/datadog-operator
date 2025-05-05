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

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestRBACManager_AddPolicyRules(t *testing.T) {
	ns := "bar"
	name := "foo"

	rule1 := rbacv1.PolicyRule{
		Verbs: []string{"POST", "GET"},
		Resources: []string{
			"pods", "deployments",
		},
		APIGroups: []string{"core/v1"},
	}

	role1 := &rbacv1.Role{
		ObjectMeta: v1.ObjectMeta{
			Namespace: ns,
			Name:      "otherrole",
		},
		Rules: []rbacv1.PolicyRule{
			rule1,
		},
	}

	rule2 := rbacv1.PolicyRule{
		Verbs: []string{"POST", "GET"},
		Resources: []string{
			"deploymenrs",
		},
		APIGroups: []string{"app/v1"},
	}

	role2 := &rbacv1.Role{
		ObjectMeta: v1.ObjectMeta{
			Namespace: ns,
			Name:      name + "role",
		},
		Rules: []rbacv1.PolicyRule{
			rule2,
		},
	}

	testScheme := runtime.NewScheme()
	testScheme.AddKnownTypes(v2alpha1.GroupVersion, &v2alpha1.DatadogAgent{})
	storeOptions := &store.StoreOptions{
		Scheme: testScheme,
	}

	owner := &v2alpha1.DatadogAgent{
		ObjectMeta: v1.ObjectMeta{
			Namespace: ns,
			Name:      name,
		},
	}

	type args struct {
		namespace string
		roleName  string
		saName    string
		policies  []rbacv1.PolicyRule
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
				namespace: ns,
				saName:    name + "sa",
				roleName:  name + "role",
				policies: []rbacv1.PolicyRule{
					rule1,
				},
			},
			wantErr: false,
			validateFunc: func(t *testing.T, store *store.Store) {
				if _, found := store.Get(kubernetes.RolesKind, ns, name+"role"); !found {
					t.Errorf("missing Role %s/%s", ns, name+"role")
				}

				if _, found := store.Get(kubernetes.RoleBindingKind, ns, name+"role"); !found {
					t.Errorf("missing RoleBinding %s/%s", ns, name+"role")
				}
			},
		},
		{
			name:  "another Role already exist",
			store: store.NewStore(owner, storeOptions).AddOrUpdateStore(kubernetes.RolesKind, role1),
			args: args{
				namespace: ns,
				saName:    name + "sa",
				roleName:  name + "role",
				policies: []rbacv1.PolicyRule{
					rule1,
				},
			},
			wantErr: false,
			validateFunc: func(t *testing.T, store *store.Store) {
				if _, found := store.Get(kubernetes.RolesKind, ns, name+"role"); !found {
					t.Errorf("missing Role %s/%s", ns, name+"role")
				}

				if _, found := store.Get(kubernetes.RoleBindingKind, ns, name+"role"); !found {
					t.Errorf("missing RoleBinding %s/%s", ns, name+"role")
				}
			},
		},
		{
			name:  "update existing Role",
			store: store.NewStore(owner, storeOptions).AddOrUpdateStore(kubernetes.RolesKind, role2),
			args: args{
				namespace: ns,
				saName:    name + "sa",
				roleName:  name + "role",
				policies: []rbacv1.PolicyRule{
					rule1,
				},
			},
			wantErr: false,
			validateFunc: func(t *testing.T, store *store.Store) {
				obj, found := store.Get(kubernetes.RolesKind, ns, name+"role")
				if !found {
					t.Errorf("missing Role %s/%s", ns, name+"role")
				}
				role, ok := obj.(*rbacv1.Role)
				if !ok || len(role.Rules) != 2 {
					t.Errorf("missing Rule in Role %s/%s", ns, name+"role")
				}

				if _, found := store.Get(kubernetes.RoleBindingKind, ns, name+"role"); !found {
					t.Errorf("missing RoleBinding %s/%s", ns, name+"role")
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &rbacManagerImpl{
				store: tt.store,
			}
			if err := m.AddPolicyRules(tt.args.namespace, tt.args.roleName, tt.args.saName, tt.args.policies); (err != nil) != tt.wantErr {
				t.Errorf("RBACManager.AddPolicyRules() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.validateFunc != nil {
				tt.validateFunc(t, tt.store)
			}
		})
	}
}

func TestRBACManager_AddClusterPolicyRules(t *testing.T) {
	name := "foo"
	ns := "bar"

	rule1 := rbacv1.PolicyRule{
		Verbs: []string{"POST", "GET"},
		Resources: []string{
			"pods", "deployments",
		},
		APIGroups: []string{"core/v1"},
	}

	role1 := &rbacv1.ClusterRole{
		ObjectMeta: v1.ObjectMeta{
			Name: "otherrole",
		},
		Rules: []rbacv1.PolicyRule{
			rule1,
		},
	}

	testScheme := runtime.NewScheme()
	testScheme.AddKnownTypes(v2alpha1.GroupVersion, &v2alpha1.DatadogAgent{})
	storeOptions := &store.StoreOptions{
		Scheme: testScheme,
	}

	owner := &v2alpha1.DatadogAgent{
		ObjectMeta: v1.ObjectMeta{
			Namespace: ns,
			Name:      name,
		},
	}

	type args struct {
		namespace string
		roleName  string
		saName    string
		policies  []rbacv1.PolicyRule
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
				namespace: ns,
				saName:    name + "sa",
				roleName:  name + "role",
				policies: []rbacv1.PolicyRule{
					rule1,
				},
			},
			wantErr: false,
			validateFunc: func(t *testing.T, store *store.Store) {
				if _, found := store.Get(kubernetes.ClusterRolesKind, "", name+"role"); !found {
					t.Errorf("missing ClusterRole %s", name+"role")
				}

				if _, found := store.Get(kubernetes.ClusterRoleBindingKind, "", name+"role"); !found {
					t.Errorf("missing ClusterRoleBinding %s", name+"role")
				}
			},
		},
		{
			name:  "another ClusterRole already exist",
			store: store.NewStore(owner, storeOptions).AddOrUpdateStore(kubernetes.RolesKind, role1),
			args: args{
				namespace: ns,
				saName:    name + "sa",
				roleName:  name + "role",
				policies: []rbacv1.PolicyRule{
					rule1,
				},
			},
			wantErr: false,
			validateFunc: func(t *testing.T, store *store.Store) {
				if _, found := store.Get(kubernetes.ClusterRolesKind, "", name+"role"); !found {
					t.Errorf("missing ClusterRole %s", name+"role")
				}

				if _, found := store.Get(kubernetes.ClusterRoleBindingKind, "", name+"role"); !found {
					t.Errorf("missing ClusterRoleBinding %s", name+"role")
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &rbacManagerImpl{
				store: tt.store,
			}
			if err := m.AddClusterPolicyRules(tt.args.namespace, tt.args.roleName, tt.args.saName, tt.args.policies); (err != nil) != tt.wantErr {
				t.Errorf("RBACManager.AddClusterPolicyRules() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.validateFunc != nil {
				tt.validateFunc(t, tt.store)
			}
		})
	}
}

func TestRBACManager_AddServiceAccountAnnotations(t *testing.T) {
	ns := "bar"
	name := "foo"

	testScheme := runtime.NewScheme()
	testScheme.AddKnownTypes(v2alpha1.GroupVersion, &v2alpha1.DatadogAgent{})
	storeOptions := &store.StoreOptions{
		Scheme: testScheme,
	}

	owner := &v2alpha1.DatadogAgent{
		ObjectMeta: v1.ObjectMeta{
			Namespace: ns,
			Name:      name,
		},
	}

	sa := &corev1.ServiceAccount{
		ObjectMeta: v1.ObjectMeta{
			Namespace: ns,
			Name:      name + "sa",
		},
	}

	testStore := store.NewStore(owner, storeOptions).AddOrUpdateStore(kubernetes.ServiceAccountsKind, sa)

	tests := []struct {
		name         string
		store        *store.Store
		namespace    string
		saName       string
		annotations  map[string]string
		wantErr      bool
		validateFunc func(*testing.T, *store.Store)
	}{
		{
			name:        "empty annotations",
			store:       testStore,
			namespace:   ns,
			saName:      name + "sa",
			annotations: map[string]string{},
			wantErr:     false,
			validateFunc: func(t *testing.T, store *store.Store) {
				obj, found := store.Get(kubernetes.ServiceAccountsKind, ns, name+"sa")
				if !found {
					t.Errorf("missing ServiceAccount %s/%s", ns, name+"sa")
				}
				sa, ok := obj.(*corev1.ServiceAccount)
				if !ok || len(sa.Annotations) != 0 {
					t.Errorf("expected empty annotations, got %v", sa.Annotations)
				}
			},
		},
		{
			name:      "single annotation",
			store:     testStore,
			namespace: ns,
			saName:    name + "sa",
			annotations: map[string]string{
				"key1": "value1",
			},
			wantErr: false,
			validateFunc: func(t *testing.T, store *store.Store) {
				obj, found := store.Get(kubernetes.ServiceAccountsKind, ns, name+"sa")
				if !found {
					t.Errorf("missing ServiceAccount %s/%s", ns, name+"sa")
				}
				sa, ok := obj.(*corev1.ServiceAccount)
				if !ok || sa.Annotations["key1"] != "value1" {
					t.Errorf("expected annotation key1=value1, got %v", sa.Annotations)
				}
			},
		},
		{
			name:      "multiple annotations",
			store:     testStore,
			namespace: ns,
			saName:    name + "sa",
			annotations: map[string]string{
				"key1": "value1",
				"key2": "value2",
			},
			wantErr: false,
			validateFunc: func(t *testing.T, store *store.Store) {
				obj, found := store.Get(kubernetes.ServiceAccountsKind, ns, name+"sa")
				if !found {
					t.Errorf("missing ServiceAccount %s/%s", ns, name+"sa")
				}
				sa, ok := obj.(*corev1.ServiceAccount)
				if !ok || sa.Annotations["key1"] != "value1" || sa.Annotations["key2"] != "value2" {
					t.Errorf("expected annotations key1=value1, key2=value2, got %v", sa.Annotations)
				}
			},
		},
		{
			name: "duplicate annotation keys",
			store: store.NewStore(owner, storeOptions).AddOrUpdateStore(kubernetes.ServiceAccountsKind, &corev1.ServiceAccount{
				ObjectMeta: v1.ObjectMeta{
					Namespace: ns,
					Name:      name + "sa",
					Annotations: map[string]string{
						"key2": "oldValue2",
						"key3": "value3",
					},
				},
			}),
			namespace: ns,
			saName:    name + "sa",
			annotations: map[string]string{
				"key1": "value1",
				"key2": "newValue2",
			},
			wantErr: false,
			validateFunc: func(t *testing.T, store *store.Store) {
				obj, found := store.Get(kubernetes.ServiceAccountsKind, ns, name+"sa")
				if !found {
					t.Errorf("missing ServiceAccount %s/%s", ns, name+"sa")
				}
				sa, ok := obj.(*corev1.ServiceAccount)
				if !ok || sa.Annotations["key1"] != "value1" || sa.Annotations["key2"] != "newValue2" || sa.Annotations["key3"] != "value3" {
					t.Errorf("expected annotations key1=value1, key2=newValue2, key3=value3 got %v", sa.Annotations)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &rbacManagerImpl{
				store: tt.store,
			}
			if err := m.AddServiceAccountAnnotations(tt.namespace, tt.saName, tt.annotations); (err != nil) != tt.wantErr {
				t.Errorf("RBACManager.AddServiceAccountAnnotations() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.validateFunc != nil {
				tt.validateFunc(t, tt.store)
			}
		})
	}
}
