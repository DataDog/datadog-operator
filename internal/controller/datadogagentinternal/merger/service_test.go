// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package merger

import (
	"testing"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/store"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestServiceManager_AddService(t *testing.T) {
	ns := "bar"
	name1 := "foo"
	name2 := "foo2"
	portNumber1 := 1111
	portNumber2 := 2222
	serviceInternalTrafficPolicy := corev1.ServiceInternalTrafficPolicyLocal
	selector := map[string]string{
		apicommon.AgentDeploymentNameLabelKey:      name1,
		apicommon.AgentDeploymentComponentLabelKey: ns,
	}
	ports1 := []corev1.ServicePort{
		{
			Protocol:   corev1.ProtocolUDP,
			TargetPort: intstr.FromInt(portNumber1),
			Port:       int32(portNumber1),
			Name:       name1,
		},
	}
	ports2 := []corev1.ServicePort{
		{
			Protocol:   corev1.ProtocolUDP,
			TargetPort: intstr.FromInt(portNumber2),
			Port:       int32(portNumber2),
			Name:       name2,
		},
	}
	existingService := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns,
			Name:      name2,
		},
		Spec: corev1.ServiceSpec{
			Ports: ports1,
		},
	}

	testScheme := runtime.NewScheme()
	testScheme.AddKnownTypes(v1alpha1.GroupVersion, &v1alpha1.DatadogAgentInternal{})
	storeOptions := &store.StoreOptions{
		Scheme: testScheme,
	}

	owner := &v1alpha1.DatadogAgentInternal{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns,
			Name:      name1,
		},
	}

	type args struct {
		namespace string
		name      string
		selector  map[string]string
		ports     []corev1.ServicePort
		itp       *corev1.ServiceInternalTrafficPolicyType
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
				selector:  selector,
				ports:     ports1,
				itp:       &serviceInternalTrafficPolicy,
			},
			wantErr: false,
			validateFunc: func(t *testing.T, store *store.Store) {
				if _, found := store.Get(kubernetes.ServicesKind, ns, name1); !found {
					t.Errorf("missing Service %s/%s", ns, name1)
				}
			},
		},
		{
			name:  "another Service already exists",
			store: store.NewStore(owner, storeOptions).AddOrUpdateStore(kubernetes.ServicesKind, &existingService),
			args: args{
				namespace: ns,
				name:      name1,
				selector:  selector,
				ports:     ports1,
				itp:       &serviceInternalTrafficPolicy,
			},
			wantErr: false,
			validateFunc: func(t *testing.T, store *store.Store) {
				if _, found := store.Get(kubernetes.ServicesKind, ns, name1); !found {
					t.Errorf("missing Service %s/%s", ns, name1)
				}
			},
		},
		{
			name:  "update existing NetworkPolicy",
			store: store.NewStore(owner, storeOptions).AddOrUpdateStore(kubernetes.ServicesKind, &existingService),
			args: args{
				namespace: ns,
				name:      name2,
				selector:  selector,
				ports:     ports2,
				itp:       &serviceInternalTrafficPolicy,
			},
			wantErr: false,
			validateFunc: func(t *testing.T, store *store.Store) {
				obj, found := store.Get(kubernetes.ServicesKind, ns, name2)
				if !found {
					t.Errorf("missing Service %s/%s", ns, name2)
				}
				service, ok := obj.(*corev1.Service)
				if !ok || len(service.Spec.Ports) != 2 {
					t.Errorf("missing Port in Service %s/%s", ns, name2)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &serviceManagerImpl{
				store: tt.store,
			}
			if err := m.AddService(tt.args.name, tt.args.namespace, tt.args.selector, tt.args.ports, tt.args.itp); (err != nil) != tt.wantErr {
				t.Errorf("ServiceManager.AddService() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.validateFunc != nil {
				tt.validateFunc(t, tt.store)
			}
		})
	}
}
