// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package merger

import (
	"testing"

	"github.com/DataDog/datadog-operator/controllers/datadogagent/dependencies"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_secretManagerImpl_AddSecret(t *testing.T) {
	secretNs := "foo"
	secretName := "bar"

	secret1 := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: secretNs,
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
		store        *dependencies.Store
		args         args
		wantErr      bool
		validateFunc func(*testing.T, *dependencies.Store)
	}{
		{
			name:  "empty Store",
			store: dependencies.NewStore(nil),
			args: args{
				secretNamespace: secretNs,
				secretName:      secretName,
				key:             "key",
				value:           "secret-value",
			},
			wantErr: false,
			validateFunc: func(t *testing.T, store *dependencies.Store) {
				if _, found := store.Get(kubernetes.SecretsKind, secretNs, secretName); !found {
					t.Errorf("missing Secret %s/%s", secretNs, secretName)
				}
			},
		},
		{
			name:  "secret already exist",
			store: dependencies.NewStore(nil).AddOrUpdateStore(kubernetes.SecretsKind, secret1),
			args: args{
				secretNamespace: secretNs,
				secretName:      secretName,
				key:             "key",
				value:           "secret-value",
			},
			wantErr: false,
			validateFunc: func(t *testing.T, store *dependencies.Store) {
				obj, found := store.Get(kubernetes.SecretsKind, secretNs, secretName)
				secret, _ := obj.(*corev1.Secret)
				if !found {
					t.Errorf("missing Secret %s/%s", secretNs, secretName)
				}
				if _, ok := secret.Data["key1"]; !ok {
					t.Errorf("default key1 not found in Secret %s/%s", secretNs, secretName)
				}
				if _, ok := secret.Data["key"]; !ok {
					t.Errorf("key not found in Secret %s/%s", secretNs, secretName)
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
			if tt.validateFunc != nil {
				tt.validateFunc(t, tt.store)
			}
		})
	}
}
