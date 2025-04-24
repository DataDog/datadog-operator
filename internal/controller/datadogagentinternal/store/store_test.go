// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package store

import (
	"context"
	"reflect"
	"testing"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	testutils "github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/testutils"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
	assert "github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	apiregistrationv1 "k8s.io/kube-aggregator/pkg/apis/apiregistration/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

func Test_buildID(t *testing.T) {
	type args struct {
		ns   string
		name string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "ns+name",
			args: args{
				ns:   "bar",
				name: "foo",
			},
			want: "bar/foo",
		},
		{
			name: "name_only",
			args: args{
				name: "foo",
			},
			want: "foo",
		},
		{
			name: "ns_only",
			args: args{
				ns: "bar",
			},
			want: "bar/",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := buildID(tt.args.ns, tt.args.name); got != tt.want {
				t.Errorf("buildID() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_buildObjectKey(t *testing.T) {
	tests := []struct {
		name string
		key  string
		want types.NamespacedName
	}{
		{
			name: "ns + name",
			key:  "bar/foo",
			want: types.NamespacedName{
				Namespace: "bar",
				Name:      "foo",
			},
		},
		{
			name: "name only",
			key:  "foo",
			want: types.NamespacedName{
				Name: "foo",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := buildObjectKey(tt.key); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("buildObjectKey() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStore_AddOrUpdate(t *testing.T) {
	dummyConfigMap1 := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "bar",
			Name:      "foo",
		},
	}

	dummyConfigMap1bis := dummyConfigMap1.DeepCopy()
	dummyConfigMap1bis.Data = map[string]string{
		"key1": "data1",
	}

	dummySecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "bar",
			Name:      "foo",
		},
		Data: map[string][]byte{
			"secret": []byte("this is a secret"),
		},
	}

	owner := &v1alpha1.DatadogAgentInternal{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "bar",
			Name:      "foo",
		},
	}

	testScheme := runtime.NewScheme()
	testScheme.AddKnownTypes(v1alpha1.GroupVersion, &v1alpha1.DatadogAgentInternal{})

	type fields struct {
		deps map[kubernetes.ObjectKind]map[string]client.Object
	}
	type args struct {
		kind kubernetes.ObjectKind
		obj  client.Object
	}
	tests := []struct {
		name           string
		fields         fields
		args           args
		validationFunc func(t *testing.T, store *Store)
		wantErr        bool
	}{
		{
			name: "add to an empty store",
			fields: fields{
				deps: make(map[kubernetes.ObjectKind]map[string]client.Object),
			},
			args: args{
				kind: kubernetes.ConfigMapKind,
				obj:  dummyConfigMap1,
			},
			validationFunc: func(t *testing.T, store *Store) {
				assert.Equal(t, len(store.deps), 1, "store len should be equal to 1, current: %d", len(store.deps))
				assert.Equal(t, store.deps[kubernetes.ConfigMapKind]["bar/foo"].GetName(), "foo", "name shoud be foo")
			},
		},
		{
			name: "update an existing configmap",
			fields: fields{
				deps: map[kubernetes.ObjectKind]map[string]client.Object{
					kubernetes.ConfigMapKind: {
						"bar/foo": dummyConfigMap1,
					},
				},
			},
			args: args{
				kind: kubernetes.ConfigMapKind,
				obj:  dummyConfigMap1bis,
			},
			validationFunc: func(t *testing.T, store *Store) {
				assert.Equal(t, len(store.deps), 1, "store len should be equal to 1, current: %d", len(store.deps))
				assert.Equal(t, store.deps[kubernetes.ConfigMapKind]["bar/foo"].GetName(), "foo", "name shoud be foo")
				assert.Equal(t, store.deps[kubernetes.ConfigMapKind]["bar/foo"], dummyConfigMap1bis, "the configmap should has been updated")
			},
		},

		{
			name: "add a second object kind",
			fields: fields{
				deps: map[kubernetes.ObjectKind]map[string]client.Object{
					kubernetes.ConfigMapKind: {
						"bar/foo": dummyConfigMap1,
					},
				},
			},
			args: args{
				kind: kubernetes.SecretsKind,
				obj:  dummySecret,
			},
			validationFunc: func(t *testing.T, store *Store) {
				assert.Equal(t, len(store.deps), 2, "store len should be equal to 2, current: %d", len(store.deps))
				assert.Equal(t, store.deps[kubernetes.ConfigMapKind]["bar/foo"].GetName(), "foo", "name shoud be foo")
				assert.Equal(t, store.deps[kubernetes.SecretsKind]["bar/foo"], dummySecret, "the secret shoud be present")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := logf.Log.WithName(t.Name())
			ds := &Store{
				deps:   tt.fields.deps,
				owner:  owner,
				scheme: testScheme,
				logger: logger,
			}
			gotErr := ds.AddOrUpdate(tt.args.kind, tt.args.obj)
			if gotErr != nil && tt.wantErr == false {
				t.Errorf("Store.AddOrUpdate() gotErr = %v, wantErr %v", gotErr, tt.wantErr)
			}
			tt.validationFunc(t, ds)
		})
	}
}

func TestStore_Get(t *testing.T) {
	dummyConfigMap1 := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "bar",
			Name:      "foo",
		},
	}

	type fields struct {
		deps map[kubernetes.ObjectKind]map[string]client.Object
	}
	type args struct {
		kind      kubernetes.ObjectKind
		namespace string
		name      string
	}
	tests := []struct {
		name      string
		fields    fields
		args      args
		want      client.Object
		wantExist bool
	}{
		{
			name: "do not exist",
			fields: fields{
				deps: map[kubernetes.ObjectKind]map[string]client.Object{},
			},
			args: args{
				kind:      kubernetes.ConfigMapKind,
				namespace: "bar",
				name:      "foo",
			},
			want:      nil,
			wantExist: false,
		},
		{
			name: "exist",
			fields: fields{
				deps: map[kubernetes.ObjectKind]map[string]client.Object{
					kubernetes.ConfigMapKind: {
						"bar/foo": dummyConfigMap1,
					},
				},
			},
			args: args{
				kind:      kubernetes.ConfigMapKind,
				namespace: "bar",
				name:      "foo",
			},
			want:      dummyConfigMap1,
			wantExist: true,
		},
		{
			name: "another configmap exist",
			fields: fields{
				deps: map[kubernetes.ObjectKind]map[string]client.Object{
					kubernetes.ConfigMapKind: {
						"bar/foo": dummyConfigMap1,
					},
				},
			},
			args: args{
				kind:      kubernetes.ConfigMapKind,
				namespace: "bar",
				name:      "not",
			},
			want:      nil,
			wantExist: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := logf.Log.WithName(t.Name())
			ds := &Store{
				deps:   tt.fields.deps,
				logger: logger,
			}
			got, gotExist := ds.Get(tt.args.kind, tt.args.namespace, tt.args.name)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Store.Get() got = %v, want %v", got, tt.want)
			}
			if gotExist != tt.wantExist {
				t.Errorf("Store.Get() got1 = %v, want %v", gotExist, tt.wantExist)
			}
		})
	}
}

func TestStore_Apply(t *testing.T) {
	dummyConfigMap1 := &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			Kind: "ConfigMap",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "bar",
			Name:      "foo",
		},
	}

	dummyConfigMap1bis := &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			Kind: "ConfigMap",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "bar",
			Name:      "foo",
		},
		Data: map[string]string{
			"data1": "value1",
		},
	}

	type fields struct {
		deps map[kubernetes.ObjectKind]map[string]client.Object
	}
	type args struct {
		ctx       context.Context
		k8sClient client.Client
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   []error
	}{
		{
			name: "nothing to apply",
			fields: fields{
				deps: map[kubernetes.ObjectKind]map[string]client.Object{},
			},
		},
		{
			name: "one ConfigMap to apply",
			fields: fields{
				deps: map[kubernetes.ObjectKind]map[string]client.Object{
					kubernetes.ConfigMapKind: {
						"bar/foo": dummyConfigMap1.DeepCopy(),
					},
				},
			},
			args: args{
				ctx:       context.TODO(),
				k8sClient: fake.NewClientBuilder().Build(),
			},
		},
		{
			name: "one ConfigMap to update",
			fields: fields{
				deps: map[kubernetes.ObjectKind]map[string]client.Object{
					kubernetes.ConfigMapKind: {
						"bar/foo": dummyConfigMap1bis.DeepCopy(),
					},
				},
			},
			args: args{
				ctx:       context.TODO(),
				k8sClient: fake.NewClientBuilder().WithObjects(dummyConfigMap1.DeepCopy()).Build(),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ds := &Store{
				deps:   tt.fields.deps,
				logger: logf.Log.WithName(t.Name()),
			}
			got := ds.Apply(tt.args.ctx, tt.args.k8sClient)
			assert.EqualValues(t, tt.want, got, "Store.Apply() = %v, want %v", got, tt.want)
		})
	}
}

func TestStore_Cleanup(t *testing.T) {
	dummyConfigMap1 := &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ConfigMap",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "bar",
			Name:      "foo",
			Labels: map[string]string{
				operatorStoreLabelKey:                  "true",
				kubernetes.AppKubernetesPartOfLabelKey: "namespace--test-dda--test",
			},
		},
	}

	dummyName := "dda-test"
	dummyNs := "namespace-test"

	s := scheme.Scheme
	s.AddKnownTypes(apiregistrationv1.SchemeGroupVersion, &apiregistrationv1.APIService{})
	s.AddKnownTypes(apiregistrationv1.SchemeGroupVersion, &apiregistrationv1.APIServiceList{})

	type fields struct {
		deps map[kubernetes.ObjectKind]map[string]client.Object
	}
	type args struct {
		ctx       context.Context
		k8sClient client.Client
		ddaNs     string
		ddaName   string
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   []error
	}{
		{
			name: "nothing to cleanup",
			fields: fields{
				deps: map[kubernetes.ObjectKind]map[string]client.Object{
					kubernetes.ConfigMapKind: {
						"bar/foo": dummyConfigMap1.DeepCopy(),
					},
				},
			},
			args: args{
				ctx:       context.TODO(),
				k8sClient: fake.NewClientBuilder().WithScheme(s).WithObjects(dummyConfigMap1.DeepCopy()).Build(),
				ddaNs:     dummyNs,
				ddaName:   dummyName,
			},
			want: nil,
		},
		{
			name: "1 object to keep",
			fields: fields{
				deps: map[kubernetes.ObjectKind]map[string]client.Object{
					kubernetes.ConfigMapKind: {
						"bar/foo": dummyConfigMap1.DeepCopy(),
					},
				},
			},
			args: args{
				ctx:       context.TODO(),
				k8sClient: fake.NewClientBuilder().WithScheme(s).WithObjects(dummyConfigMap1.DeepCopy()).Build(),
				ddaNs:     dummyNs,
				ddaName:   dummyName,
			},
			want: nil,
		},
		{
			name: "1 object to delete",
			fields: fields{
				deps: map[kubernetes.ObjectKind]map[string]client.Object{},
			},
			args: args{
				ctx:       context.TODO(),
				k8sClient: fake.NewClientBuilder().WithScheme(s).WithObjects(dummyConfigMap1.DeepCopy()).Build(),
				ddaNs:     dummyNs,
				ddaName:   dummyName,
			},
			want: nil,
		},
		{
			name: "1 object to keep from a different dda",
			fields: fields{
				deps: map[kubernetes.ObjectKind]map[string]client.Object{},
			},
			args: args{
				ctx:       context.TODO(),
				k8sClient: fake.NewClientBuilder().WithScheme(s).WithObjects(dummyConfigMap1.DeepCopy()).Build(),
				ddaNs:     "test",
				ddaName:   dummyName,
			},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ds := &Store{
				deps:   tt.fields.deps,
				logger: logf.Log.WithName(t.Name()),
				owner: &metav1.ObjectMeta{
					Name:      tt.args.ddaName,
					Namespace: tt.args.ddaNs,
				},
			}
			got := ds.Cleanup(tt.args.ctx, tt.args.k8sClient)
			assert.EqualValues(t, tt.want, got, "Store.Cleanup() = %v, want %v", got, tt.want)
		})
	}
}

func TestStore_GetOrCreate(t *testing.T) {
	dummyConfigMap1 := &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ConfigMap",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "bar",
			Name:      "foo",
			Labels: map[string]string{
				operatorStoreLabelKey: "true",
			},
		},
	}

	emptyConfigMap1 := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "bar",
			Name:      "foo",
		},
	}
	emptyNotConfigMap1 := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "bar",
			Name:      "not",
		},
	}

	type fields struct {
		deps          map[kubernetes.ObjectKind]map[string]client.Object
		supportCilium bool
	}
	type args struct {
		kind      kubernetes.ObjectKind
		namespace string
		name      string
	}
	tests := []struct {
		name      string
		fields    fields
		args      args
		want      client.Object
		wantFound bool
	}{
		{
			name: "do not exist",
			fields: fields{
				deps: map[kubernetes.ObjectKind]map[string]client.Object{},
			},
			args: args{
				kind:      kubernetes.ConfigMapKind,
				namespace: "bar",
				name:      "foo",
			},
			want:      emptyConfigMap1,
			wantFound: false,
		},
		{
			name: "exist",
			fields: fields{
				deps: map[kubernetes.ObjectKind]map[string]client.Object{
					kubernetes.ConfigMapKind: {
						"bar/foo": dummyConfigMap1,
					},
				},
			},
			args: args{
				kind:      kubernetes.ConfigMapKind,
				namespace: "bar",
				name:      "foo",
			},
			want:      dummyConfigMap1,
			wantFound: true,
		},
		{
			name: "another configmap exist",
			fields: fields{
				deps: map[kubernetes.ObjectKind]map[string]client.Object{
					kubernetes.ConfigMapKind: {
						"bar/foo": dummyConfigMap1,
					},
				},
			},
			args: args{
				kind:      kubernetes.ConfigMapKind,
				namespace: "bar",
				name:      "not",
			},
			want:      emptyNotConfigMap1,
			wantFound: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ds := &Store{
				deps:          tt.fields.deps,
				supportCilium: tt.fields.supportCilium,
			}
			got, got1 := ds.GetOrCreate(tt.args.kind, tt.args.namespace, tt.args.name)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Store.GetOrCreate() got = %v,\nwant %v", got, tt.want)
			}
			if got1 != tt.wantFound {
				t.Errorf("Store.GetOrCreate() got1 = %v, want %v", got1, tt.wantFound)
			}
		})
	}
}

func TestStore_DeleteAll(t *testing.T) {
	testConfigMap1 := &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ConfigMap",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns1",
			Name:      "some_name",
			Labels: map[string]string{
				operatorStoreLabelKey: "true",
			},
		},
	}

	testConfigMap2 := &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ConfigMap",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns2",
			Name:      "another_name",
			Labels: map[string]string{
				operatorStoreLabelKey: "true",
			},
		},
	}

	testStore := map[kubernetes.ObjectKind]map[string]client.Object{
		kubernetes.ConfigMapKind: {
			"ns1/some_name":    testConfigMap1,
			"ns2/another_name": testConfigMap2,
		},
	}

	// ConfigMap not included in testStore
	testConfigMap3 := &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ConfigMap",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns3",
			Name:      "some_name",
			Labels: map[string]string{
				operatorStoreLabelKey: "true",
			},
		},
	}

	tests := []struct {
		name                          string
		dependenciesStore             map[kubernetes.ObjectKind]map[string]client.Object
		existingObjects               []client.Object
		objectsExpectedToBeDeleted    []client.Object
		objectsExpectedNotToBeDeleted []client.Object
	}{
		{
			name:              "deletes all the objects in the store",
			dependenciesStore: testStore,
			existingObjects: []client.Object{
				testConfigMap1,
				testConfigMap2,
			},
			objectsExpectedToBeDeleted: []client.Object{
				testConfigMap1,
				testConfigMap2,
			},
		},
		{
			name:              "does not delete objects that are not in the store",
			dependenciesStore: testStore,
			existingObjects: []client.Object{
				testConfigMap1,
				testConfigMap2,
				testConfigMap3, // Not in dependenciesStore
			},
			objectsExpectedToBeDeleted: []client.Object{
				testConfigMap1,
				testConfigMap2,
			},
			objectsExpectedNotToBeDeleted: []client.Object{
				testConfigMap3, // Not in dependenciesStore
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			k8sClient := fake.NewClientBuilder().
				WithScheme(testutils.TestScheme()).
				WithObjects(test.existingObjects...).
				Build()

			store := &Store{
				deps: test.dependenciesStore,
			}

			errs := store.DeleteAll(context.TODO(), k8sClient)
			assert.Empty(t, errs)

			for _, expectedToBeDeleted := range test.objectsExpectedToBeDeleted {
				err := k8sClient.Get(
					context.TODO(),
					client.ObjectKey{
						Namespace: expectedToBeDeleted.GetNamespace(),
						Name:      expectedToBeDeleted.GetName(),
					},
					&corev1.ConfigMap{}, // Adapt according to test input objects
				)
				assert.True(t, errors.IsNotFound(err))
			}

			for _, expectedToExist := range test.objectsExpectedNotToBeDeleted {
				err := k8sClient.Get(
					context.TODO(),
					client.ObjectKey{
						Namespace: expectedToExist.GetNamespace(),
						Name:      expectedToExist.GetName(),
					},
					&corev1.ConfigMap{}, // Adapt according to test input objects
				)
				assert.NoError(t, err)
			}
		})
	}
}
