// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package global

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/fake"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/store"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
	"github.com/DataDog/datadog-operator/pkg/testutils"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

func Test_ApplyFIPSConfig(t *testing.T) {
	logger := logf.Log.WithName("Test_ApplyFIPSConfig")

	testScheme := runtime.NewScheme()
	testScheme.AddKnownTypes(v2alpha1.GroupVersion, &v2alpha1.DatadogAgent{})
	storeOptions := &store.StoreOptions{
		Scheme: testScheme,
	}

	agentContainer := &corev1.Container{
		Name:  string(apicommon.CoreAgentContainerName),
		Image: string("gcr.io/datadoghq/operator:7.64.0"),
	}
	processAgentContainer := &corev1.Container{
		Name:  string(apicommon.ProcessAgentContainerName),
		Image: string("gcr.io/datadoghq/operator:7.64.0"),
	}
	systemProbeContainer := &corev1.Container{
		Name:  string(apicommon.SystemProbeContainerName),
		Image: string("gcr.io/datadoghq/operator:7.64.0"),
	}

	customConfig := `global
    presetenv DD_FIPS_LOCAL_ADDRESS 127.0.0.1
    log 127.0.0.1 local0
    ssl-default-server-ciphers ECDHE-RSA-AES128-GCM-SHA256:ECDHE-RSA-AES256-GCM-SHA384:ECDHE-ECDSA-AES128-GCM-SHA256:ECDHE-ECDSA-AES256-GCM-SHA384:AES128-GCM-SHA256:AES256-GCM-SHA384:!aNULL:!eNULL:!EXPORT
    ssl-default-bind-options no-sslv3 no-tlsv10 no-tlsv11 no-tlsv13
    ssl-default-server-options no-sslv3 no-tlsv10 no-tlsv11 no-tlsv13
    default-path config

# Some sane defaults
defaults
    log     global
    option  dontlognull
    retries 3
    option  redispatch
    timeout client 5s
    timeout server 5s
    timeout connect 5s
    default-server verify required ca-file ca-certificates.crt check inter 10s resolvers my-dns init-addr none resolve-prefer ipv4`

	tests := []struct {
		name            string
		dda             *v2alpha1.DatadogAgent
		existingManager func() *fake.PodTemplateManagers
		want            func(t testing.TB, manager *fake.PodTemplateManagers, store *store.Store)
	}{
		{
			name: "FIPS mode enabled",
			dda: testutils.NewDatadogAgentBuilder().
				WithFIPS(v2alpha1.FIPSConfig{
					ModeEnabled: apiutils.NewBoolPointer(true),
				}).
				BuildWithDefaults(),
			existingManager: func() *fake.PodTemplateManagers {
				return fake.NewPodTemplateManagers(t, corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{*agentContainer, *processAgentContainer, *systemProbeContainer},
					},
				})
			},
			want: func(t testing.TB, mgr *fake.PodTemplateManagers, store *store.Store) {
				checkFIPSImages(t, mgr)
			},
		},
		{
			name: "FIPS proxy enabled (deprecated parameter)",
			dda: testutils.NewDatadogAgentBuilder().
				WithFIPS(v2alpha1.FIPSConfig{
					Enabled: apiutils.NewBoolPointer(true),
				}).
				BuildWithDefaults(),
			existingManager: func() *fake.PodTemplateManagers {
				return fake.NewPodTemplateManagers(t, corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{*agentContainer, *processAgentContainer, *systemProbeContainer},
					},
				})
			},
			want: func(t testing.TB, mgr *fake.PodTemplateManagers, store *store.Store) {
				// fips env var
				checkFIPSContainerEnvVars(t, mgr)
				// fips port
				checkFIPSPort(t, mgr, int32(9803))

				// component env var
				checkComponentContainerEnvVars(t, mgr, 9803)

				// volume
				checkVolume(t, mgr, false)
				// volume mounts
				checkVolumeMounts(t, mgr, false)
			},
		},
		{
			name: "FIPS proxy enabled",
			dda: testutils.NewDatadogAgentBuilder().
				WithFIPS(v2alpha1.FIPSConfig{
					ProxyEnabled: apiutils.NewBoolPointer(true),
				}).
				BuildWithDefaults(),
			existingManager: func() *fake.PodTemplateManagers {
				return fake.NewPodTemplateManagers(t, corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{*agentContainer, *processAgentContainer, *systemProbeContainer},
					},
				})
			},
			want: func(t testing.TB, mgr *fake.PodTemplateManagers, store *store.Store) {
				// fips env var
				checkFIPSContainerEnvVars(t, mgr)
				// fips port
				checkFIPSPort(t, mgr, int32(9803))

				// component env var
				checkComponentContainerEnvVars(t, mgr, 9803)

				// volume
				checkVolume(t, mgr, false)
				// volume mounts
				checkVolumeMounts(t, mgr, false)
			},
		},
		{
			name: "FIPS proxy enabled, custom image",
			dda: testutils.NewDatadogAgentBuilder().
				WithFIPS(v2alpha1.FIPSConfig{
					ProxyEnabled: apiutils.NewBoolPointer(true),
					Image: &v2alpha1.AgentImageConfig{
						Name: "registry/custom:tag",
					},
				}).
				BuildWithDefaults(),
			existingManager: func() *fake.PodTemplateManagers {
				return fake.NewPodTemplateManagers(t, corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{*agentContainer, *processAgentContainer, *systemProbeContainer},
					},
				})
			},
			want: func(t testing.TB, mgr *fake.PodTemplateManagers, store *store.Store) {
				// fips env var
				checkFIPSContainerEnvVars(t, mgr)
				// fips port
				checkFIPSPort(t, mgr, int32(9803))

				// component env var
				checkComponentContainerEnvVars(t, mgr, 9803)

				// volume
				checkVolume(t, mgr, false)
				// volume mounts
				checkVolumeMounts(t, mgr, false)

				assert.Equal(t, "registry/custom:tag", mgr.PodTemplateSpec().Spec.Containers[3].Image)
			},
		},
		{
			name: "FIPS proxy enabled, custom port",
			dda: testutils.NewDatadogAgentBuilder().
				WithFIPS(v2alpha1.FIPSConfig{
					ProxyEnabled: apiutils.NewBoolPointer(true),
					Port:         apiutils.NewInt32Pointer(2),
				}).
				BuildWithDefaults(),
			existingManager: func() *fake.PodTemplateManagers {
				return fake.NewPodTemplateManagers(t, corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{*agentContainer, *processAgentContainer, *systemProbeContainer},
					},
				})
			},
			want: func(t testing.TB, mgr *fake.PodTemplateManagers, store *store.Store) {
				// fips env var
				checkFIPSContainerEnvVars(t, mgr)
				// fips port
				checkFIPSPort(t, mgr, int32(2))

				// component env var
				checkComponentContainerEnvVars(t, mgr, 2)

				// volume
				checkVolume(t, mgr, false)
				// volume mounts
				checkVolumeMounts(t, mgr, false)
			},
		},
		{
			name: "FIPS proxy enabled, custom config - config map",
			dda: testutils.NewDatadogAgentBuilder().
				WithFIPS(v2alpha1.FIPSConfig{
					ProxyEnabled: apiutils.NewBoolPointer(true),
					CustomFIPSConfig: &v2alpha1.CustomConfig{
						ConfigMap: &v2alpha1.ConfigMapConfig{
							Name: "foo",
							Items: []corev1.KeyToPath{
								{
									Key:  "foo-key",
									Path: "foo-path",
								},
							},
						},
						ConfigData: apiutils.NewStringPointer("{foo:bar}"),
					},
				}).
				BuildWithDefaults(),
			existingManager: func() *fake.PodTemplateManagers {
				return fake.NewPodTemplateManagers(t, corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{*agentContainer, *processAgentContainer, *systemProbeContainer},
					},
				})
			},
			want: func(t testing.TB, mgr *fake.PodTemplateManagers, store *store.Store) {
				// fips env var
				checkFIPSContainerEnvVars(t, mgr)
				// fips port
				checkFIPSPort(t, mgr, int32(9803))

				// component env var
				checkComponentContainerEnvVars(t, mgr, 9803)

				// volume
				checkVolume(t, mgr, true)
				// volume mounts
				checkVolumeMounts(t, mgr, true)
			},
		},
		{
			name: "FIPS proxy enabled, custom config - config data",
			dda: testutils.NewDatadogAgentBuilder().
				WithFIPS(v2alpha1.FIPSConfig{
					ProxyEnabled: apiutils.NewBoolPointer(true),
					CustomFIPSConfig: &v2alpha1.CustomConfig{
						ConfigData: apiutils.NewStringPointer(customConfig),
					},
				}).
				BuildWithDefaults(),
			existingManager: func() *fake.PodTemplateManagers {
				return fake.NewPodTemplateManagers(t, corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{*agentContainer, *processAgentContainer, *systemProbeContainer},
					},
				})
			},
			want: func(t testing.TB, mgr *fake.PodTemplateManagers, store *store.Store) {
				// fips env var
				checkFIPSContainerEnvVars(t, mgr)
				// fips port
				checkFIPSPort(t, mgr, int32(9803))

				// component env var
				checkComponentContainerEnvVars(t, mgr, 9803)

				// volume
				checkVolume(t, mgr, false)
				// volume mounts
				checkVolumeMounts(t, mgr, true)

				// check configMap exists in store and config data value set for correct key
				cmData := map[string]string{"datadog-fips-proxy.cfg": customConfig}
				cm, ok := store.Get(kubernetes.ConfigMapKind, "", "-fips-config")
				assert.True(t, ok)
				configMap, ok := cm.(*corev1.ConfigMap)
				assert.True(t, ok)
				assert.Equal(t, cmData, configMap.Data)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			podTemplateManager := tt.existingManager()
			store := store.NewStore(tt.dda, storeOptions)
			resourcesManager := feature.NewResourceManagers(store)

			applyFIPSConfig(logger, podTemplateManager, tt.dda, resourcesManager)

			tt.want(t, podTemplateManager, store)
		})
	}
}

func getExpectedComponentContainerEnvVars(port int) []*corev1.EnvVar {
	return []*corev1.EnvVar{
		{
			Name:  DDFIPSEnabled,
			Value: "true",
		},
		{
			Name:  DDFIPSPortRangeStart,
			Value: strconv.Itoa(port),
		},
		{
			Name:  DDFIPSUseHTTPS,
			Value: "false",
		},
		{
			Name:  DDFIPSLocalAddress,
			Value: "127.0.0.1",
		},
	}
}

func getExpectedFIPSVolume(customConfig bool) []*corev1.Volume {
	vol := []*corev1.Volume{
		{
			Name: FIPSProxyCustomConfigVolumeName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: fmt.Sprintf(FIPSProxyCustomConfigMapName, ""),
					},
					Items: []corev1.KeyToPath{
						{
							Key:  FIPSProxyCustomConfigFileName,
							Path: FIPSProxyCustomConfigFileName,
						},
					},
				},
			},
		},
	}
	if customConfig {
		vol[0].VolumeSource.ConfigMap.LocalObjectReference.Name = "foo"
		vol[0].VolumeSource.ConfigMap.Items[0].Key = "foo-key"
		vol[0].VolumeSource.ConfigMap.Items[0].Path = "foo-path"
	}
	return vol
}

func getExpectedFIPSVolumeMounts() []*corev1.VolumeMount {
	vm := getFIPSVolumeMount()
	volMount := []*corev1.VolumeMount{&vm}
	return volMount
}

func getFIPSVolumeMount() corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      FIPSProxyCustomConfigVolumeName,
		MountPath: FIPSProxyCustomConfigMountPath,
		SubPath:   FIPSProxyCustomConfigFileName,
		ReadOnly:  true,
	}
}

func checkFIPSImages(t testing.TB, mgr *fake.PodTemplateManagers) {
	for _, container := range mgr.PodTemplateSpec().Spec.Containers {
		assert.True(t, strings.HasSuffix(container.Image, "-fips"), "Container %s has image %s", container.Name, container.Image)
	}
}

func checkFIPSContainerEnvVars(t testing.TB, mgr *fake.PodTemplateManagers) {
	fipsEnvVars := mgr.PodTemplateSpec().Spec.Containers[3].Env
	expectedEnvVars := corev1.EnvVar{
		Name:  DDFIPSLocalAddress,
		Value: "127.0.0.1",
	}
	assert.Contains(t, fipsEnvVars, expectedEnvVars)
}

func checkComponentContainerEnvVars(t testing.TB, mgr *fake.PodTemplateManagers, port int) {
	coreAgentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommon.CoreAgentContainerName]
	processAgentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommon.ProcessAgentContainerName]
	systemProbeEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommon.SystemProbeContainerName]
	expectedEnvVars := getExpectedComponentContainerEnvVars(port)

	assert.True(t, apiutils.IsEqualStruct(coreAgentEnvVars, expectedEnvVars), "Core agent container envvars \ndiff = %s", cmp.Diff(coreAgentEnvVars, expectedEnvVars))
	assert.True(t, apiutils.IsEqualStruct(processAgentEnvVars, expectedEnvVars), "Process agent container envvars \ndiff = %s", cmp.Diff(processAgentEnvVars, expectedEnvVars))
	assert.True(t, apiutils.IsEqualStruct(systemProbeEnvVars, nil), "System probe container envvars \ndiff = %s", cmp.Diff(systemProbeEnvVars, nil))
}

func checkVolume(t testing.TB, mgr *fake.PodTemplateManagers, customConfig bool) {
	volumes := mgr.VolumeMgr.Volumes
	expectedVolumes := getExpectedFIPSVolume(customConfig)
	assert.True(t, apiutils.IsEqualStruct(volumes, expectedVolumes), "Volumes \ndiff = %s", cmp.Diff(volumes, expectedVolumes))
}

func checkVolumeMounts(t testing.TB, mgr *fake.PodTemplateManagers, customConfig bool) {
	coreAgentVolumeMounts := mgr.VolumeMountMgr.VolumeMountsByC[apicommon.CoreAgentContainerName]
	processAgentVolumeMounts := mgr.VolumeMountMgr.VolumeMountsByC[apicommon.ProcessAgentContainerName]
	systemProbeVolumeMounts := mgr.VolumeMountMgr.VolumeMountsByC[apicommon.SystemProbeContainerName]
	fipsVolumeMounts := mgr.VolumeMountMgr.VolumeMountsByC[apicommon.FIPSProxyContainerName]
	expectedVolumeMounts := getExpectedFIPSVolumeMounts()
	if !customConfig {
		expectedVolumeMounts = nil
	}

	assert.True(t, apiutils.IsEqualStruct(coreAgentVolumeMounts, expectedVolumeMounts), "Core agent volume mounts \ndiff = %s", cmp.Diff(coreAgentVolumeMounts, expectedVolumeMounts))
	assert.True(t, apiutils.IsEqualStruct(processAgentVolumeMounts, nil), "Process agent volume mounts \ndiff = %s", cmp.Diff(processAgentVolumeMounts, nil))
	assert.True(t, apiutils.IsEqualStruct(systemProbeVolumeMounts, nil), "System probe volume mounts \ndiff = %s", cmp.Diff(systemProbeVolumeMounts, nil))
	assert.True(t, apiutils.IsEqualStruct(fipsVolumeMounts, expectedVolumeMounts), "FIPS proxy volume mounts \ndiff = %s", cmp.Diff(fipsVolumeMounts, expectedVolumeMounts))
}

func checkFIPSPort(t testing.TB, mgr *fake.PodTemplateManagers, startingPort int32) {
	fipsPorts := mgr.PodTemplateSpec().Spec.Containers[3].Ports
	expectedPorts := makeExpectedFIPSPortList(startingPort)
	assert.ElementsMatch(t, fipsPorts, expectedPorts)
}

func makeExpectedFIPSPortList(startingNumber int32) []corev1.ContainerPort {
	return []corev1.ContainerPort{
		{
			Name:          "port-0",
			ContainerPort: startingNumber,
			Protocol:      corev1.ProtocolTCP,
		},
		{
			Name:          "port-1",
			ContainerPort: startingNumber + 1,
			Protocol:      corev1.ProtocolTCP,
		},
		{
			Name:          "port-2",
			ContainerPort: startingNumber + 2,
			Protocol:      corev1.ProtocolTCP,
		},
		{
			Name:          "port-3",
			ContainerPort: startingNumber + 3,
			Protocol:      corev1.ProtocolTCP,
		},
		{
			Name:          "port-4",
			ContainerPort: startingNumber + 4,
			Protocol:      corev1.ProtocolTCP,
		},
		{
			Name:          "port-5",
			ContainerPort: startingNumber + 5,
			Protocol:      corev1.ProtocolTCP,
		},
		{
			Name:          "port-6",
			ContainerPort: startingNumber + 6,
			Protocol:      corev1.ProtocolTCP,
		},
		{
			Name:          "port-7",
			ContainerPort: startingNumber + 7,
			Protocol:      corev1.ProtocolTCP,
		},
		{
			Name:          "port-8",
			ContainerPort: startingNumber + 8,
			Protocol:      corev1.ProtocolTCP,
		},
		{
			Name:          "port-9",
			ContainerPort: startingNumber + 9,
			Protocol:      corev1.ProtocolTCP,
		},
		{
			Name:          "port-10",
			ContainerPort: startingNumber + 10,
			Protocol:      corev1.ProtocolTCP,
		},
		{
			Name:          "port-11",
			ContainerPort: startingNumber + 11,
			Protocol:      corev1.ProtocolTCP,
		},
		{
			Name:          "port-12",
			ContainerPort: startingNumber + 12,
			Protocol:      corev1.ProtocolTCP,
		},
		{
			Name:          "port-13",
			ContainerPort: startingNumber + 13,
			Protocol:      corev1.ProtocolTCP,
		},
		{
			Name:          "port-14",
			ContainerPort: startingNumber + 14,
			Protocol:      corev1.ProtocolTCP,
		},
	}
}
