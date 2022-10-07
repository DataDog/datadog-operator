// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package merger

import (
	"fmt"
	"testing"

	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/dependencies"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"

	securityv1 "github.com/openshift/api/security/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestPodSecurityManager_AddSecurityContextConstraints(t *testing.T) {
	ns := "bar"
	newSCCName := "foo"
	existingSCCName := "foo2"

	newSCC := &securityv1.SecurityContextConstraints{
		Users: []string{
			fmt.Sprintf("system:serviceaccount:%s:%s", ns, newSCCName),
		},
		Priority: apiutils.NewInt32Pointer(8),
		AllowedCapabilities: []corev1.Capability{
			"SYS_ADMIN",
			"SYS_RESOURCE",
			"SYS_PTRACE",
			"NET_ADMIN",
			"NET_BROADCAST",
			"NET_RAW",
			"IPC_LOCK",
			"CHOWN",
			"AUDIT_CONTROL",
			"AUDIT_READ",
		},
		AllowHostDirVolumePlugin: true,
		AllowHostIPC:             true,
		AllowPrivilegedContainer: false,
		FSGroup: securityv1.FSGroupStrategyOptions{
			Type: securityv1.FSGroupStrategyMustRunAs,
		},
	}

	existingSCC := securityv1.SecurityContextConstraints{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns,
			Name:      existingSCCName,
		},
		Users: []string{
			fmt.Sprintf("system:serviceaccount:%s:%s", ns, existingSCCName),
		},
		AllowHostDirVolumePlugin: false,
		FSGroup: securityv1.FSGroupStrategyOptions{
			Type: securityv1.FSGroupStrategyMustRunAs,
		},
		Volumes: []securityv1.FSType{
			securityv1.FSTypeConfigMap,
			securityv1.FSTypeDownwardAPI,
			securityv1.FSTypeEmptyDir,
			securityv1.FSTypePersistentVolumeClaim,
			securityv1.FSProjected,
			securityv1.FSTypeSecret,
		},
	}

	testScheme := runtime.NewScheme()
	testScheme.AddKnownTypes(v2alpha1.GroupVersion, &v2alpha1.DatadogAgent{})
	storeOptions := &dependencies.StoreOptions{
		Scheme: testScheme,
	}

	owner := &v2alpha1.DatadogAgent{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns,
			Name:      newSCCName,
		},
	}

	type args struct {
		namespace string
		name      string
		scc       *securityv1.SecurityContextConstraints
	}
	tests := []struct {
		name         string
		store        *dependencies.Store
		args         args
		wantErr      bool
		validateFunc func(*testing.T, *dependencies.Store)
	}{
		{
			name:  "empty store",
			store: dependencies.NewStore(owner, storeOptions),
			args: args{
				namespace: ns,
				name:      newSCCName,
				scc:       newSCC,
			},
			wantErr: false,
			validateFunc: func(t *testing.T, store *dependencies.Store) {
				if _, found := store.Get(kubernetes.SecurityContextConstraintsKind, ns, newSCCName); !found {
					t.Errorf("missing SecurityContextConstraints %s/%s", ns, newSCCName)
				}
			},
		},
		{
			name:  "another SecurityContextConstraints already exists",
			store: dependencies.NewStore(owner, storeOptions).AddOrUpdateStore(kubernetes.SecurityContextConstraintsKind, &existingSCC),
			args: args{
				namespace: ns,
				name:      newSCCName,
				scc:       newSCC,
			},
			wantErr: false,
			validateFunc: func(t *testing.T, store *dependencies.Store) {
				if _, found := store.Get(kubernetes.SecurityContextConstraintsKind, ns, newSCCName); !found {
					t.Errorf("missing SecurityContextConstraints %s/%s", ns, newSCCName)
				}
			},
		},
		{
			name:  "update existing SecurityContextConstraints",
			store: dependencies.NewStore(owner, storeOptions).AddOrUpdateStore(kubernetes.SecurityContextConstraintsKind, &existingSCC),
			args: args{
				namespace: ns,
				name:      existingSCCName,
				scc:       newSCC,
			},
			wantErr: false,
			validateFunc: func(t *testing.T, store *dependencies.Store) {
				obj, found := store.Get(kubernetes.SecurityContextConstraintsKind, ns, existingSCCName)
				if !found {
					t.Errorf("missing SecurityContextConstraints %s/%s", ns, existingSCCName)
				}
				scc, ok := obj.(*securityv1.SecurityContextConstraints)
				if !ok || !scc.AllowHostDirVolumePlugin {
					t.Errorf("AllowHostDirVolumePlugin not updated in SecurityContextConstraints %s/%s", ns, existingSCCName)
				}
				if len(scc.Volumes) != 6 {
					t.Errorf("Volumes changed in SecurityContextConstraints %s/%s", ns, existingSCCName)
				}
				if len(scc.AllowedCapabilities) != 10 {
					t.Errorf("AllowedCapabilities not added in SecurityContextConstraints %s/%s", ns, existingSCCName)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &podSecurityManagerImpl{
				store: tt.store,
			}
			if err := m.AddSecurityContextConstraints(tt.args.name, tt.args.namespace, tt.args.scc); (err != nil) != tt.wantErr {
				t.Errorf("PodSecurityManager.AddSecurityContextConstraints() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.validateFunc != nil {
				tt.validateFunc(t, tt.store)
			}
		})
	}
}
