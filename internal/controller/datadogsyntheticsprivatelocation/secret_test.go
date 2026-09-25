// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogsyntheticsprivatelocation

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	ctrutils "github.com/DataDog/datadog-operator/pkg/controller/utils"
)

const testBaseConfig = `{"id":"pl-id-123","accessKey":"ak","secretAccessKey":"sak","publicKey":"pk","privateKey":"prk","site":"datadoghq.com"}`

func configWithInstance(t *testing.T, config *datadoghqv1alpha1.DatadogSPLWorkerConfig, overrideJSON string) map[string]interface{} {
	t.Helper()
	merged, err := mergeWorkerConfig([]byte(testBaseConfig), config, overrideJSON, "datadoghq.com")
	require.NoError(t, err)
	var cfg map[string]interface{}
	require.NoError(t, json.Unmarshal(merged, &cfg))
	return cfg
}

func Test_mergeWorkerConfigBase(t *testing.T) {
	cfg := configWithInstance(t, nil, "")
	assert.Equal(t, "ak", cfg["accessKey"])
	assert.Equal(t, "pl-id-123", cfg["id"])
	assert.Equal(t, "datadoghq.com", cfg["site"])
}

func Test_mergeWorkerConfigTypedOverrides(t *testing.T) {
	config := &datadoghqv1alpha1.DatadogSPLWorkerConfig{
		Concurrency:                 ptr.To(int32(5)),
		EnableStatusProbes:          ptr.To(true),
		StatusProbesPort:            ptr.To(int32(8081)),
		ProxyTestRequestsBypassList: []string{"example.com"},
		ReportMetrics:               ptr.To(false),
	}
	cfg := configWithInstance(t, config, "")
	assert.Equal(t, float64(5), cfg["concurrency"])
	assert.Equal(t, true, cfg["enableStatusProbes"])
	assert.Equal(t, float64(8081), cfg["statusProbesPort"])
	assert.Equal(t, []interface{}{"example.com"}, cfg["proxyTestRequestsBypassList"])
	assert.Equal(t, false, cfg["reportMetrics"])
	// API-managed keys are untouched.
	assert.Equal(t, "ak", cfg["accessKey"])
}

func Test_mergeWorkerConfigRawOverrideLastWins(t *testing.T) {
	cfg := configWithInstance(t, nil, `{"concurrency": 10, "tags": ["foo"]}`)
	assert.Equal(t, float64(10), cfg["concurrency"])
	assert.Equal(t, []interface{}{"foo"}, cfg["tags"])
}

func Test_mergeWorkerConfigRawOverrideCannotTouchSite(t *testing.T) {
	cfg := configWithInstance(t, nil, `{"site": "evil.example.com"}`)
	assert.Equal(t, "datadoghq.com", cfg["site"])
}

func Test_mergeWorkerConfigApiManagedKeysRejected(t *testing.T) {
	for _, key := range []string{"accessKey", "secretAccessKey", "publicKey", "privateKey", "id"} {
		_, err := mergeWorkerConfig([]byte(testBaseConfig), nil, `{"`+key+`": "hacked"}`, "datadoghq.com")
		assert.Error(t, err, key)
		assert.True(t, ctrutils.IsPermanentAPIError(err), key)
	}
}

func Test_mergeWorkerConfigInvalidBase(t *testing.T) {
	_, err := mergeWorkerConfig([]byte(`{invalid`), nil, "", "datadoghq.com")
	assert.Error(t, err)
	assert.True(t, ctrutils.IsPermanentAPIError(err))
}

func Test_mergeWorkerConfigInvalidOverrideJSON(t *testing.T) {
	_, err := mergeWorkerConfig([]byte(testBaseConfig), nil, `{invalid`, "datadoghq.com")
	assert.Error(t, err)
	assert.True(t, ctrutils.IsPermanentAPIError(err))
}

func newTestSecretWithData(data string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "my-pl-config", Namespace: "default"},
		Data:       map[string][]byte{datadoghqv1alpha1.DatadogSPLConfigSecretDataKey: []byte(data)},
	}
}

func Test_reconcileConfigSecretCreate(t *testing.T) {
	s := newSchemeWithSPL(t)
	c := fake.NewClientBuilder().WithScheme(s).Build()
	instance := newTestInstance()

	err := reconcileConfigSecret(context.Background(), c, s, instance, []byte(testBaseConfig), "datadoghq.com")
	require.NoError(t, err)

	secret := &corev1.Secret{}
	require.NoError(t, c.Get(context.Background(), client.ObjectKey{Name: "my-pl-config", Namespace: "default"}, secret))
	assert.Equal(t, "my-pl-config", secret.Name)
	assert.Contains(t, secret.Data, datadoghqv1alpha1.DatadogSPLConfigSecretDataKey)
	var cfg map[string]interface{}
	require.NoError(t, json.Unmarshal(secret.Data[datadoghqv1alpha1.DatadogSPLConfigSecretDataKey], &cfg))
	assert.Equal(t, "ak", cfg["accessKey"])
	assert.Equal(t, "datadoghq.com", cfg["site"])
	assertOwnedByInstance(t, secret, instance)
}

func Test_reconcileConfigSecretUpdateKeepsManagedKeys(t *testing.T) {
	s := newSchemeWithSPL(t)
	c := fake.NewClientBuilder().WithScheme(s).WithObjects(newTestSecretWithData(testBaseConfig)).Build()
	instance := newTestInstance()
	instance.Spec.Worker = &datadoghqv1alpha1.DatadogSPLWorker{
		Config: &datadoghqv1alpha1.DatadogSPLWorkerConfig{
			Concurrency: ptr.To(int32(7)),
		},
	}

	// No new base config: the existing Secret is the base.
	err := reconcileConfigSecret(context.Background(), c, s, instance, nil, "datadoghq.com")
	require.NoError(t, err)

	updated := &corev1.Secret{}
	require.NoError(t, c.Get(context.Background(), client.ObjectKey{Name: "my-pl-config", Namespace: "default"}, updated))
	var cfg map[string]interface{}
	require.NoError(t, json.Unmarshal(updated.Data[datadoghqv1alpha1.DatadogSPLConfigSecretDataKey], &cfg))
	assert.Equal(t, float64(7), cfg["concurrency"])
	assert.Equal(t, "ak", cfg["accessKey"])
}

func Test_reconcileConfigSecretOverrideAnnotation(t *testing.T) {
	s := newSchemeWithSPL(t)
	c := fake.NewClientBuilder().WithScheme(s).WithObjects(newTestSecretWithData(testBaseConfig)).Build()
	instance := newTestInstance()
	instance.Spec.Worker = &datadoghqv1alpha1.DatadogSPLWorker{
		Config: &datadoghqv1alpha1.DatadogSPLWorkerConfig{
			Concurrency: ptr.To(int32(7)),
		},
	}
	instance.Annotations = map[string]string{
		datadoghqv1alpha1.DatadogSPLWorkerConfigOverrideAnnotation: `{"concurrency": 12, "logFormat": "json"}`,
	}

	err := reconcileConfigSecret(context.Background(), c, s, instance, nil, "datadoghq.com")
	require.NoError(t, err)

	updated := &corev1.Secret{}
	require.NoError(t, c.Get(context.Background(), client.ObjectKey{Name: "my-pl-config", Namespace: "default"}, updated))
	var cfg map[string]interface{}
	require.NoError(t, json.Unmarshal(updated.Data[datadoghqv1alpha1.DatadogSPLConfigSecretDataKey], &cfg))
	assert.Equal(t, float64(12), cfg["concurrency"])
	assert.Equal(t, "json", cfg["logFormat"])
	assert.Equal(t, "ak", cfg["accessKey"])
}

func Test_reconcileConfigSecretMissingAndNoBase(t *testing.T) {
	s := newSchemeWithSPL(t)
	c := fake.NewClientBuilder().WithScheme(s).Build()
	instance := newTestInstance()

	// Neither an existing Secret nor a creation-time config: unrecoverable.
	err := reconcileConfigSecret(context.Background(), c, s, instance, nil, "datadoghq.com")
	assert.Error(t, err)
}

func Test_reconcileConfigSecretMissingDataKey(t *testing.T) {
	s := newSchemeWithSPL(t)
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "my-pl-config", Namespace: "default"},
		Data:       map[string][]byte{"other": []byte("data")},
	}
	c := fake.NewClientBuilder().WithScheme(s).WithObjects(secret).Build()
	instance := newTestInstance()

	err := reconcileConfigSecret(context.Background(), c, s, instance, nil, "datadoghq.com")
	assert.Error(t, err)
}
