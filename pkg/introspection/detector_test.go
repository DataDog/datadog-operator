// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package introspection

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

func testScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = corev1.AddToScheme(s)
	return s
}

func node(name string, labels map[string]string) *corev1.Node {
	return &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: name, Labels: labels}}
}

func clientWith(objs ...client.Object) client.Client {
	return fake.NewClientBuilder().WithScheme(testScheme()).WithObjects(objs...).Build()
}

func TestDetector_detect(t *testing.T) {
	eksLabels := map[string]string{"eks.amazonaws.com/nodegroup-image": "ami-123"}
	ocpLabels := map[string]string{kubernetes.OpenShiftProviderLabel: "rhcos"}

	autopilotPlatform := kubernetes.NewPlatformInfoFromVersionMaps(nil, map[string]string{"AllowlistedV2Workload": "auto.gke.io/v1"}, nil)
	autopilotPlatformV1 := kubernetes.NewPlatformInfoFromVersionMaps(nil, map[string]string{"AllowlistedWorkload": "auto.gke.io/v1"}, nil)

	tests := []struct {
		name         string
		platformInfo kubernetes.PlatformInfo
		nodeName     string
		apiReader    client.Reader
		nodeClient   client.Client
		wantProvider string
		wantSource   string
		wantNil      bool
	}{
		{
			// Stage 0: GKE Autopilot is platform-API detected and wins over node
			// labels (nodes are COS, which the node stages would resolve to default).
			name:         "stage 0 platform-API GKE Autopilot (v2 CRD) wins over node labels",
			platformInfo: autopilotPlatform,
			nodeName:     "self",
			apiReader:    clientWith(node("self", map[string]string{kubernetes.GKEProviderLabel: kubernetes.GKECosType})),
			wantProvider: kubernetes.GKEAutopilotProvider,
			wantSource:   sourcePlatform,
		},
		{
			name:         "stage 0 platform-API GKE Autopilot detected via v1 CRD",
			platformInfo: autopilotPlatformV1,
			nodeName:     "self",
			apiReader:    clientWith(node("self", nil)),
			wantProvider: kubernetes.GKEAutopilotProvider,
			wantSource:   sourcePlatform,
		},
		{
			name:         "stage 1 operator-node EKS is authoritative",
			nodeName:     "self",
			apiReader:    clientWith(node("self", eksLabels)),
			nodeClient:   clientWith(node("self", eksLabels)),
			wantProvider: kubernetes.EKSCloudProvider,
			wantSource:   sourceOwnNode,
		},
		{
			name:         "stage 1 operator-node default published even when unlabeled",
			nodeName:     "self",
			apiReader:    clientWith(node("self", nil)),
			wantProvider: kubernetes.DefaultProvider,
			wantSource:   sourceOwnNode,
		},
		{
			name:         "stage 2 fallback when operator-node read fails",
			nodeName:     "missing",
			apiReader:    clientWith(), // own node absent -> Get NotFound
			nodeClient:   clientWith(node("n1", ocpLabels)),
			wantProvider: "openshift-rhcos",
			wantSource:   sourceNodeList,
		},
		{
			name:         "stage 2 used when no node name",
			nodeName:     "",
			apiReader:    clientWith(),
			nodeClient:   clientWith(node("n1", eksLabels)),
			wantProvider: kubernetes.EKSCloudProvider,
			wantSource:   sourceNodeList,
		},
		{
			name:       "no node name and no node client yields no detection",
			nodeName:   "",
			apiReader:  clientWith(),
			nodeClient: nil,
			wantNil:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &Detector{platformInfo: tt.platformInfo, apiReader: tt.apiReader, nodeClient: tt.nodeClient, nodeName: tt.nodeName, logger: logf.Log}
			got := d.detect(context.Background())
			if tt.wantNil {
				assert.Nil(t, got)
				return
			}
			assert.NotNil(t, got)
			assert.Equal(t, tt.wantProvider, got.Provider)
			assert.Equal(t, tt.wantSource, got.Source)
		})
	}
}

func TestDetector_ProviderAndStartedAt(t *testing.T) {
	d := &Detector{apiReader: clientWith(), logger: logf.Log}

	// Before any detection: unambiguously not-ready.
	p, ok := d.Provider()
	assert.False(t, ok)
	assert.Equal(t, "", p)

	// Before Start: not started.
	_, started := d.StartedAt()
	assert.False(t, started)

	// After publishing a default detection, Provider reports detected=true even
	// though the value is "default".
	d.publish(&detection{Provider: kubernetes.DefaultProvider, Source: sourceOwnNode})
	p, ok = d.Provider()
	assert.True(t, ok)
	assert.Equal(t, kubernetes.DefaultProvider, p)
}

func TestDetector_StartSetsStartedAtAndPublishes(t *testing.T) {
	d := &Detector{
		apiReader: clientWith(node("self", map[string]string{"eks.amazonaws.com/nodegroup-image": "ami"})),
		nodeName:  "self",
		logger:    logf.Log,
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { _ = d.Start(ctx); close(done) }()

	assert.Eventually(t, func() bool {
		p, ok := d.Provider()
		return ok && p == kubernetes.EKSCloudProvider
	}, 2*time.Second, 10*time.Millisecond)

	_, started := d.StartedAt()
	assert.True(t, started)

	cancel()
	<-done
}
