// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagent

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/utils/ptr"

	v1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/store"
	agenttestutils "github.com/DataDog/datadog-operator/internal/controller/datadogagent/testutils"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
	"github.com/DataDog/datadog-operator/pkg/testutils"
)

func TestAddDDASharedDependenciesIncludesProfileAPMPort(t *testing.T) {
	dda := testutils.NewInitializedDatadogAgentBuilder("default", "datadog").WithAPMEnabled(false).BuildWithDefaults()
	baseDDAI := testDDAIWithSpec("datadog", dda.Namespace, &dda.Spec)

	profileSpec := dda.Spec.DeepCopy()
	enableAPMForDependencyTest(profileSpec, nil)
	profileDDAI := testDDAIWithSpec("datadog-profile", dda.Namespace, profileSpec)

	depsStore := newDependencyTestStore(dda)
	managers := feature.NewResourceManagers(depsStore)

	err := (&Reconciler{}).addDDASharedDependencies(dda, []*v1alpha1.DatadogAgentInternal{baseDDAI, profileDDAI}, nil, managers)
	require.NoError(t, err)

	obj, found := depsStore.Get(kubernetes.ServicesKind, "default", "datadog-agent")
	require.True(t, found)
	service := obj.(*corev1.Service)

	apmPort := findServicePortByName(service.Spec.Ports, constants.DefaultApmPortName)
	require.NotNil(t, apmPort)
	assert.Equal(t, corev1.ProtocolTCP, apmPort.Protocol)
	assert.Equal(t, int32(constants.DefaultApmPort), apmPort.Port)
	assert.Equal(t, intstr.FromInt(int(constants.DefaultApmPort)), apmPort.TargetPort)
}

func TestAddDDASharedDependenciesRejectsConflictingProfilePorts(t *testing.T) {
	dda := testutils.NewInitializedDatadogAgentBuilder("default", "datadog").WithAPMEnabled(false).BuildWithDefaults()

	profileSpecA := dda.Spec.DeepCopy()
	enableAPMForDependencyTest(profileSpecA, ptr.To[int32](8126))

	profileSpecB := dda.Spec.DeepCopy()
	enableAPMForDependencyTest(profileSpecB, ptr.To[int32](9126))

	depsStore := newDependencyTestStore(dda)
	managers := feature.NewResourceManagers(depsStore)

	err := (&Reconciler{}).addDDASharedDependencies(dda, []*v1alpha1.DatadogAgentInternal{
		testDDAIWithSpec("datadog-profile-a", dda.Namespace, profileSpecA),
		testDDAIWithSpec("datadog-profile-b", dda.Namespace, profileSpecB),
	}, nil, managers)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "default/datadog-profile-b DDA shared dependencies failed")
	assert.Contains(t, err.Error(), "port \"traceport\" conflicts")
	assert.Contains(t, err.Error(), "service port conflict")
}

// Use the chosen Service port while keeping each DDAI's node ports unchanged.
func TestAddDDASharedDependenciesUsesResolvedProfilePort(t *testing.T) {
	for _, baseAPMEnabled := range []bool{false, true} {
		for _, hostNetwork := range []bool{false, true} {
			t.Run(fmt.Sprintf("base APM enabled=%t/host network=%t", baseAPMEnabled, hostNetwork), func(t *testing.T) {
				dda := testutils.NewInitializedDatadogAgentBuilder("default", "datadog").WithAPMEnabled(baseAPMEnabled).BuildWithDefaults()
				sharedSpec := dda.Spec.DeepCopy()
				sharedSpec.Features.APM.HostPortConfig = &v2alpha1.HostPortConfig{Enabled: ptr.To(true), Port: ptr.To[int32](9126)}
				baseDDAI := testDDAIWithSpec("datadog", dda.Namespace, sharedSpec)

				explicitProfile := dda.Spec.DeepCopy()
				enableAPMForDependencyTest(explicitProfile, ptr.To[int32](9126))
				defaultedProfile := dda.Spec.DeepCopy()
				enableAPMForDependencyTest(defaultedProfile, nil)
				defaultedProfile.Override = map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
					v2alpha1.NodeAgentComponentName: {HostNetwork: ptr.To(hostNetwork)},
				}
				before := defaultedProfile.DeepCopy()
				depsStore := newDependencyTestStore(dda)
				// The explicit default spec must work regardless of list order.
				ddais := []*v1alpha1.DatadogAgentInternal{
					testDDAIWithSpec("datadog-defaulted", dda.Namespace, defaultedProfile),
					testDDAIWithSpec("datadog-explicit", dda.Namespace, explicitProfile),
					baseDDAI,
				}
				err := (&Reconciler{}).addDDASharedDependencies(dda, ddais, &baseDDAI.Spec, feature.NewResourceManagers(depsStore))
				assert.Equal(t, before, &ddais[0].Spec)
				if hostNetwork {
					// Numeric targets differ between host-networked and normal Pods.
					require.ErrorContains(t, err, "service port conflict")
					return
				}
				require.NoError(t, err)
				obj, found := depsStore.Get(kubernetes.ServicesKind, "default", "datadog-agent")
				require.True(t, found)
				service := obj.(*corev1.Service)
				port := findServicePortByName(service.Spec.Ports, constants.DefaultApmPortName)
				require.NotNil(t, port)
				assert.Equal(t, int32(9126), port.Port)
				assert.Equal(t, intstr.FromInt(int(constants.DefaultApmPort)), port.TargetPort)
			})
		}
	}
}

func TestAddDDASharedDependenciesDoesNotMutateDDAISpec(t *testing.T) {
	dda := testutils.NewInitializedDatadogAgentBuilder("default", "datadog").BuildWithDefaults()
	ddai := testDDAIWithSpec("datadog", dda.Namespace, &dda.Spec)

	require.NotNil(t, ddai.Spec.Features.ServiceDiscovery)
	require.Nil(t, ddai.Spec.Features.ServiceDiscovery.Enabled)

	depsStore := newDependencyTestStore(dda)
	managers := feature.NewResourceManagers(depsStore)

	err := (&Reconciler{}).addDDASharedDependencies(dda, []*v1alpha1.DatadogAgentInternal{ddai}, nil, managers)
	require.NoError(t, err)

	assert.Nil(t, ddai.Spec.Features.ServiceDiscovery.Enabled)
}

func newDependencyTestStore(dda *v2alpha1.DatadogAgent) *store.Store {
	return store.NewStore(dda, &store.StoreOptions{
		PlatformInfo: kubernetes.NewPlatformInfoFromVersionMaps(&version.Info{GitVersion: "1.32.0"}, nil, nil),
		Scheme:       agenttestutils.TestScheme(),
	})
}

func enableAPMForDependencyTest(spec *v2alpha1.DatadogAgentSpec, hostPort *int32) {
	spec.Features.APM.Enabled = ptr.To(true)
	spec.Features.APM.HostPortConfig = &v2alpha1.HostPortConfig{
		Enabled: ptr.To(hostPort != nil),
		Port:    ptr.To[int32](constants.DefaultApmPort),
	}
	if hostPort != nil {
		spec.Features.APM.HostPortConfig.Port = hostPort
	}
	spec.Features.APM.UnixDomainSocketConfig = &v2alpha1.UnixDomainSocketConfig{
		Enabled: ptr.To(false),
		Path:    ptr.To(common.DogstatsdAPMSocketHostPath + "/" + common.APMSocketName),
	}
}

func testDDAIWithSpec(name, namespace string, spec *v2alpha1.DatadogAgentSpec) *v1alpha1.DatadogAgentInternal {
	return &v1alpha1.DatadogAgentInternal{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: *spec.DeepCopy(),
	}
}

func findServicePortByName(ports []corev1.ServicePort, name string) *corev1.ServicePort {
	for i := range ports {
		if ports[i].Name == name {
			return &ports[i]
		}
	}
	return nil
}
