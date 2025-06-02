// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package merger

import (
	"testing"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/store"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

func Test_secretManagerImpl_AddSecret(t *testing.T) {
	logger := logf.Log.WithName(t.Name())
	secretNs := "foo"
	secretName := "bar"
	secretAnnotations := map[string]string{
		"checksum/default-custom-config": "0fe60b5fsweqe3224werwer",
	}
	owner := &v1alpha1.DatadogAgentInternal{
		ObjectMeta: v1.ObjectMeta{
			Namespace: secretNs,
			Name:      secretName,
		},
	}

	testScheme := runtime.NewScheme()
	testScheme.AddKnownTypes(v1alpha1.GroupVersion, &v1alpha1.DatadogAgentInternal{})
	storeOptions := &store.StoreOptions{
		Scheme: testScheme,
	}

	secret1 := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:        secretName,
			Namespace:   secretNs,
			Annotations: secretAnnotations,
		},
		Data: map[string][]byte{
			"key1": []byte("defaultvalue"),
		},
	}

	type args struct {
		secretNamespace string
		secretName      string
		key             string
		value           string
	}
	tests := []struct {
		name         string
		store        *store.Store
		args         args
		wantErr      bool
		validateFunc func(*testing.T, *store.Store)
	}{
		{
			name:  "empty Store",
			store: store.NewStore(owner, storeOptions),
			args: args{
				secretNamespace: secretNs,
				secretName:      secretName,
				key:             "key",
				value:           "secret-value",
			},
			wantErr: false,
			validateFunc: func(t *testing.T, store *store.Store) {
				if _, found := store.Get(kubernetes.SecretsKind, secretNs, secretName); !found {
					t.Errorf("missing Secret %s/%s", secretNs, secretName)
				}
			},
		},
		{
			name:  "secret already exist",
			store: store.NewStore(owner, storeOptions).AddOrUpdateStore(kubernetes.SecretsKind, secret1),
			args: args{
				secretNamespace: secretNs,
				secretName:      secretName,
				key:             "key",
				value:           "secret-value",
			},
			wantErr: false,
			validateFunc: func(t *testing.T, store *store.Store) {
				obj, found := store.Get(kubernetes.SecretsKind, secretNs, secretName)
				secret, ok := obj.(*corev1.Secret)
				if !ok {
					t.Fatalf("unable to cast the obj to a Secret %s/%s", secretNs, secretName)
				}
				if !found {
					t.Fatalf("missing Secret %s/%s", secretNs, secretName)
				}
				if _, ok := secret.Data["key1"]; !ok {
					t.Errorf("default key1 not found in Secret %s/%s", secretNs, secretName)
				}
				if _, ok := secret.Data["key"]; !ok {
					t.Errorf("key not found in Secret %s/%s", secretNs, secretName)
				}
				if _, ok := secret.Annotations[object.GetChecksumAnnotationKey("default")]; !ok {
					t.Errorf("missing extraMetadata in Secret %s/%s", secretNs, secretName)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &secretManagerImpl{
				store: tt.store,
			}
			if err := m.AddSecret(tt.args.secretNamespace, tt.args.secretName, tt.args.key, tt.args.value); (err != nil) != tt.wantErr {
				t.Errorf("secretManagerImpl.AddSecret() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err := m.AddAnnotations(logger, tt.args.secretNamespace, tt.args.secretName, secretAnnotations); (err != nil) != tt.wantErr {
				t.Errorf("secretManagerImpl.AddAnnotations() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.validateFunc != nil {
				tt.validateFunc(t, tt.store)
			}
		})
	}
}
