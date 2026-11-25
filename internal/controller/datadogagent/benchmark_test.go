// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagent

import (
	"context"
	"fmt"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/pkg/agentprofile"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/comparison"
)

func newBenchScheme() *k8sruntime.Scheme {
	sch := k8sruntime.NewScheme()
	_ = scheme.AddToScheme(sch)
	_ = v1alpha1.AddToScheme(sch)
	_ = v2alpha1.AddToScheme(sch)
	_ = corev1.AddToScheme(sch)
	_ = apiextensionsv1.AddToScheme(sch)
	return sch
}

func newBenchBaseDDAI() *v1alpha1.DatadogAgentInternal {
	envVars := make([]corev1.EnvVar, 0, 15)
	for i := range 15 {
		envVars = append(envVars, corev1.EnvVar{
			Name:  fmt.Sprintf("DD_CUSTOM_VAR_%02d", i),
			Value: fmt.Sprintf("value-%02d", i),
		})
	}
	ddai := &v1alpha1.DatadogAgentInternal{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "datadoghq.com/v1alpha1",
			Kind:       "DatadogAgentInternal",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "datadog",
			Namespace: "datadog-agent",
			Labels: map[string]string{
				apicommon.DatadogAgentNameLabelKey: "datadog",
				"app.kubernetes.io/managed-by":     "datadog-operator",
				"env":                              "prod",
			},
			Annotations: map[string]string{
				"agent.datadoghq.com/provider": "aws",
			},
			Finalizers: []string{constants.DatadogAgentInternalFinalizer},
		},
		Spec: v2alpha1.DatadogAgentSpec{
			Global: &v2alpha1.GlobalConfig{
				ClusterName: ptr.To("prod-cluster-01"),
				Site:        ptr.To("datadoghq.com"),
				Registry:    ptr.To("gcr.io/datadoghq"),
				Tags:        []string{"env:prod", "region:us-east-1", "cluster:prod-cluster-01"},
				Credentials: &v2alpha1.DatadogCredentials{
					APISecret: &v2alpha1.SecretConfig{SecretName: "datadog-secret", KeyName: "api-key"},
					AppSecret: &v2alpha1.SecretConfig{SecretName: "datadog-secret", KeyName: "app-key"},
				},
				CommonLabels: map[string]string{
					"team": "container-platform",
					"env":  "prod",
				},
			},
			Features: &v2alpha1.DatadogFeatures{
				APM:                 &v2alpha1.APMFeatureConfig{Enabled: ptr.To(true)},
				LogCollection:       &v2alpha1.LogCollectionFeatureConfig{Enabled: ptr.To(true), ContainerCollectAll: ptr.To(true)},
				LiveProcessCollection: &v2alpha1.LiveProcessCollectionFeatureConfig{Enabled: ptr.To(true)},
				LiveContainerCollection: &v2alpha1.LiveContainerCollectionFeatureConfig{Enabled: ptr.To(true)},
				NPM:                 &v2alpha1.NPMFeatureConfig{Enabled: ptr.To(true)},
				USM:                 &v2alpha1.USMFeatureConfig{Enabled: ptr.To(true)},
				CWS:                 &v2alpha1.CWSFeatureConfig{Enabled: ptr.To(true)},
				CSPM:                &v2alpha1.CSPMFeatureConfig{Enabled: ptr.To(true)},
				Dogstatsd:           &v2alpha1.DogstatsdFeatureConfig{OriginDetectionEnabled: ptr.To(true)},
				AdmissionController: &v2alpha1.AdmissionControllerFeatureConfig{Enabled: ptr.To(true)},
				KubeStateMetricsCore: &v2alpha1.KubeStateMetricsCoreFeatureConfig{Enabled: ptr.To(true)},
				OrchestratorExplorer: &v2alpha1.OrchestratorExplorerFeatureConfig{Enabled: ptr.To(true)},
				RemoteConfiguration: &v2alpha1.RemoteConfigurationFeatureConfig{Enabled: ptr.To(true)},
				SBOM:                &v2alpha1.SBOMFeatureConfig{Enabled: ptr.To(true)},
				EventCollection:     &v2alpha1.EventCollectionFeatureConfig{CollectKubernetesEvents: ptr.To(true)},
			},
			Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
				v2alpha1.NodeAgentComponentName: {
					Image: &v2alpha1.AgentImageConfig{Tag: "7.78.0"},
					Env:   envVars,
					Labels: map[string]string{
						"component": "node-agent",
					},
					Annotations: map[string]string{
						"prometheus.io/scrape": "true",
					},
					Tolerations: []corev1.Toleration{
						{Operator: corev1.TolerationOpExists},
					},
					Containers: map[apicommon.AgentContainerName]*v2alpha1.DatadogAgentGenericContainer{
						apicommon.CoreAgentContainerName: {
							Resources: &corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("200m"),
									corev1.ResourceMemory: resource.MustParse("256Mi"),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("1000m"),
									corev1.ResourceMemory: resource.MustParse("1Gi"),
								},
							},
						},
						apicommon.TraceAgentContainerName: {
							Resources: &corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("100m"),
									corev1.ResourceMemory: resource.MustParse("200Mi"),
								},
							},
						},
					},
				},
				v2alpha1.ClusterAgentComponentName: {
					Replicas: ptr.To(int32(2)),
					Image:    &v2alpha1.AgentImageConfig{Tag: "7.78.0"},
					Env: []corev1.EnvVar{
						{Name: "DD_LEADER_ELECTION", Value: "true"},
						{Name: "DD_CLUSTER_CHECKS_ENABLED", Value: "true"},
					},
				},
			},
		},
	}
	_, _ = comparison.SetMD5GenerationAnnotation(&ddai.ObjectMeta, ddai.Spec, constants.MD5DDAIDeploymentAnnotationKey)
	return ddai
}

func newBenchProfiles(total int) []*v1alpha1.DatadogAgentProfile {
	defaultProfile := agentprofile.DefaultProfile()
	profiles := make([]*v1alpha1.DatadogAgentProfile, 0, total)
	profiles = append(profiles, &defaultProfile)

	for i := 1; i < total; i++ {
		name := fmt.Sprintf("profile-pool-%02d", i)
		p := &v1alpha1.DatadogAgentProfile{
			ObjectMeta: metav1.ObjectMeta{
				Name:            name,
				Namespace:       "datadog-agent",
				UID:             types.UID(fmt.Sprintf("uid-profile-%02d", i)),
				ResourceVersion: fmt.Sprintf("%d", 1000+i),
			},
			Spec: v1alpha1.DatadogAgentProfileSpec{
				ProfileAffinity: &v1alpha1.ProfileAffinity{
					ProfileNodeAffinity: []corev1.NodeSelectorRequirement{
						{
							Key:      "node-pool",
							Operator: corev1.NodeSelectorOpIn,
							Values:   []string{fmt.Sprintf("pool-%02d", i)},
						},
					},
				},
				Config: &v2alpha1.DatadogAgentSpec{
					Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
						v2alpha1.NodeAgentComponentName: {
							Env: []corev1.EnvVar{
								{Name: "DD_CUSTOM_VAR_00", Value: fmt.Sprintf("override-%02d", i)},
								{Name: "DD_PROFILE_POOL", Value: fmt.Sprintf("pool-%02d", i)},
							},
							Containers: map[apicommon.AgentContainerName]*v2alpha1.DatadogAgentGenericContainer{
								apicommon.CoreAgentContainerName: {
									Resources: &corev1.ResourceRequirements{
										Limits: corev1.ResourceList{
											corev1.ResourceCPU:    resource.MustParse("2000m"),
											corev1.ResourceMemory: resource.MustParse("2Gi"),
										},
									},
								},
							},
						},
					},
				},
			},
		}
		profiles = append(profiles, p)
	}
	return profiles
}

func newBenchReconciler(b *testing.B, objs ...client.Object) *Reconciler {
	b.Helper()
	sch := newBenchScheme()
	crd, err := getDDAICRDFromConfig(sch)
	if err != nil {
		b.Fatalf("failed to load CRD: %v", err)
	}
	allObjs := append([]client.Object{crd}, objs...)
	fakeClient := fake.NewClientBuilder().WithScheme(sch).WithObjects(allObjs...).Build()
	recorder := record.NewBroadcaster().NewRecorder(sch, corev1.EventSource{Component: "bench"})
	fm, err := newFieldManager(context.Background(), fakeClient, sch, getDDAIGVK())
	if err != nil {
		b.Fatalf("failed to create field manager: %v", err)
	}
	return &Reconciler{
		client:       fakeClient,
		scheme:       sch,
		recorder:     recorder,
		fieldManager: fm,
	}
}

func BenchmarkApplyProfilesToDDAISpec_20Profiles_SteadyState(b *testing.B) {
	ctx := context.Background()
	baseDDAI := newBenchBaseDDAI()
	defaultDDAI := baseDDAI.DeepCopy()
	profiles := newBenchProfiles(20)
	r := newBenchReconciler(b, baseDDAI)

	// Warm up once
	if _, err := r.applyProfilesToDDAISpec(ctx, baseDDAI, defaultDDAI, profiles); err != nil {
		b.Fatalf("warmup failed: %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		ddais, err := r.applyProfilesToDDAISpec(ctx, baseDDAI, defaultDDAI, profiles)
		if err != nil {
			b.Fatalf("applyProfilesToDDAISpec failed: %v", err)
		}
		if len(ddais) != 20 {
			b.Fatalf("expected 20 ddais, got %d", len(ddais))
		}
	}
}

func BenchmarkApplyProfilesToDDAISpec_20Profiles_Cold(b *testing.B) {
	ctx := context.Background()
	baseDDAI := newBenchBaseDDAI()
	profiles := newBenchProfiles(20)
	r := newBenchReconciler(b, baseDDAI)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; b.Loop(); i++ {
		// Mutate a field each iteration so any steady-state cache misses and we measure cold merge throughput.
		base := baseDDAI.DeepCopy()
		base.Spec.Override[v2alpha1.NodeAgentComponentName].Env[0].Value = fmt.Sprintf("iter-%d", i)
		_, _ = comparison.SetMD5GenerationAnnotation(&base.ObjectMeta, base.Spec, constants.MD5DDAIDeploymentAnnotationKey)
		def := base.DeepCopy()

		ddais, err := r.applyProfilesToDDAISpec(ctx, base, def, profiles)
		if err != nil {
			b.Fatalf("applyProfilesToDDAISpec failed: %v", err)
		}
		if len(ddais) != 20 {
			b.Fatalf("expected 20 ddais, got %d", len(ddais))
		}
	}
}

func BenchmarkComputeProfileMerge_DefaultProfile(b *testing.B) {
	ctx := context.Background()
	baseDDAI := newBenchBaseDDAI()
	defaultProfile := agentprofile.DefaultProfile()
	r := newBenchReconciler(b, baseDDAI)

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := r.computeProfileMerge(ctx, baseDDAI, &defaultProfile); err != nil {
			b.Fatalf("computeProfileMerge failed: %v", err)
		}
	}
}

func BenchmarkComputeProfileMerge_UserProfile(b *testing.B) {
	ctx := context.Background()
	baseDDAI := newBenchBaseDDAI()
	profile := newBenchProfiles(2)[1]
	r := newBenchReconciler(b, baseDDAI)

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := r.computeProfileMerge(ctx, baseDDAI, profile); err != nil {
			b.Fatalf("computeProfileMerge failed: %v", err)
		}
	}
}

func BenchmarkSSAMergeCRD(b *testing.B) {
	ctx := context.Background()
	baseDDAI := newBenchBaseDDAI()
	profile := newBenchProfiles(2)[1]
	r := newBenchReconciler(b, baseDDAI)

	profileDDAI := baseDDAI.DeepCopy()
	setProfileSpec(profileDDAI, profile)
	_ = setProfileDDAIMeta(profileDDAI, profile)
	baseDDAI.GetObjectKind().SetGroupVersionKind(getDDAIGVK())
	profileDDAI.GetObjectKind().SetGroupVersionKind(getDDAIGVK())

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := r.ssaMergeCRD(ctx, baseDDAI, profileDDAI); err != nil {
			b.Fatalf("ssaMergeCRD failed: %v", err)
		}
	}
}
