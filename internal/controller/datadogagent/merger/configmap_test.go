// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package merger

import (
	"testing"

	"github.com/DataDog/datadog-operator/api/crds/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/store"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestConfigMapManager_AddConfigMap(t *testing.T) {
	ns := "bar"
	name1 := "foo"
	name2 := "foo2"

	cmData := map[string]string{
		"key1": "value1",
		"key2": "value2",
	}

	existingConfigMap := corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns,
			Name:      name2,
		},
		Data: map[string]string{
			"existing-key": "existing-data",
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
		namespace string
		name      string
		data      map[string]string
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
				name:      name1,
				data:      cmData,
			},
			wantErr: false,
			validateFunc: func(t *testing.T, store *store.Store) {
				if _, found := store.Get(kubernetes.ConfigMapKind, ns, name1); !found {
					t.Errorf("missing ConfigMap %s/%s", ns, name1)
				}
			},
		},
		{
			name:  "another ConfigMap already exists",
			store: store.NewStore(owner, storeOptions).AddOrUpdateStore(kubernetes.ConfigMapKind, &existingConfigMap),
			args: args{
				namespace: ns,
				name:      name1,
				data:      cmData,
			},
			wantErr: false,
			validateFunc: func(t *testing.T, store *store.Store) {
				if _, found := store.Get(kubernetes.ConfigMapKind, ns, name1); !found {
					t.Errorf("missing ConfigMap %s/%s", ns, name1)
				}
			},
		},
		{
			name:  "update existing ConfigMap",
			store: store.NewStore(owner, storeOptions).AddOrUpdateStore(kubernetes.ConfigMapKind, &existingConfigMap),
			args: args{
				namespace: ns,
				name:      name2,
				data:      cmData,
			},
			wantErr: false,
			validateFunc: func(t *testing.T, store *store.Store) {
				obj, found := store.Get(kubernetes.ConfigMapKind, ns, name2)
				if !found {
					t.Errorf("missing ConfigMap %s/%s", ns, name2)
				}
				cm, ok := obj.(*corev1.ConfigMap)
				if !ok || len(cm.Data) != 2 {
					t.Errorf("missing data in ConfigMap %s/%s", ns, name2)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &configMapManagerImpl{
				store: tt.store,
			}
			if err := m.AddConfigMap(tt.args.name, tt.args.namespace, tt.args.data); (err != nil) != tt.wantErr {
				t.Errorf("ConfigMapManager.Add=ConfigMap() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.validateFunc != nil {
				tt.validateFunc(t, tt.store)
			}
		})
	}
}
