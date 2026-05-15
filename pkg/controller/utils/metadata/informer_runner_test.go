// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package metadata

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	authorizationv1 "k8s.io/api/authorization/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

func Test_EncodeDecodeKey(t *testing.T) {
	key := EncodeKey("DatadogAgent", "datadog", "my-agent")
	if key != "DatadogAgent/datadog/my-agent" {
		t.Fatalf("EncodeKey = %q, want DatadogAgent/datadog/my-agent", key)
	}

	kind, ns, name, ok := decodeKey(key)
	if !ok || kind != "DatadogAgent" || ns != "datadog" || name != "my-agent" {
		t.Fatalf("decodeKey(%q) = (%q, %q, %q, %v), want (DatadogAgent, datadog, my-agent, true)",
			key, kind, ns, name, ok)
	}

	if _, _, _, ok := decodeKey("malformed"); ok {
		t.Errorf("decodeKey(\"malformed\") ok = true, want false")
	}
}

func Test_InformerWorkQueue_DispatchesAdd(t *testing.T) {
	var (
		gotKind, gotNS, gotName string
		called                  atomic.Int32
	)
	r := NewInformerWorkQueue(
		zap.New(zap.UseDevMode(true)),
		nil,
		1,
		0,
		func(ctx context.Context, kind, ns, name string) error {
			gotKind, gotNS, gotName = kind, ns, name
			called.Add(1)
			return nil
		},
		nil,
		nil,
	)

	go r.Run(t.Context())

	r.Enqueue("DatadogAgent", "datadog", "my-agent")

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if called.Load() == 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if called.Load() != 1 {
		t.Fatalf("processFn called %d times, want 1", called.Load())
	}
	if gotKind != "DatadogAgent" || gotNS != "datadog" || gotName != "my-agent" {
		t.Errorf("got (%q, %q, %q), want (DatadogAgent, datadog, my-agent)", gotKind, gotNS, gotName)
	}
}

func Test_InformerWorkQueue_DispatchesDelete(t *testing.T) {
	var called atomic.Int32
	r := NewInformerWorkQueue(
		zap.New(zap.UseDevMode(true)),
		nil,
		1,
		0,
		func(ctx context.Context, kind, ns, name string) error { return nil },
		func(kind, ns, name string) {
			called.Add(1)
		},
		nil,
	)

	go r.Run(t.Context())

	r.EnqueueDelete("DatadogAgent", "datadog", "my-agent")

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if called.Load() == 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if called.Load() != 1 {
		t.Fatalf("deleteFn called %d times, want 1", called.Load())
	}
}

func Test_InformerWorkQueue_HeartbeatFires(t *testing.T) {
	var called atomic.Int32
	r := NewInformerWorkQueue(
		zap.New(zap.UseDevMode(true)),
		nil,
		1,
		20*time.Millisecond,
		func(ctx context.Context, kind, ns, name string) error { return nil },
		nil,
		func(ctx context.Context) { called.Add(1) },
	)

	go r.Run(t.Context())

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if called.Load() >= 2 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if called.Load() < 2 {
		t.Fatalf("heartbeatFn called %d times, want >= 2", called.Load())
	}
}

// fakeManagerForRBAC returns a manager.Manager-shaped value whose GetClient() returns
// a fake client that always allows SARs. Only GetClient() is used by canListWatch.
type fakeManagerForRBAC struct {
	manager.Manager
	c client.Client
}

func (f *fakeManagerForRBAC) GetClient() client.Client { return f.c }

func newAllowingFakeManager(t *testing.T) *fakeManagerForRBAC {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme: %v", err)
	}
	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithInterceptorFuncs(interceptor.Funcs{
			Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
				if sar, ok := obj.(*authorizationv1.SelfSubjectAccessReview); ok {
					sar.Status.Allowed = true
					return nil
				}
				return c.Create(ctx, obj, opts...)
			},
		}).
		Build()
	return &fakeManagerForRBAC{c: c}
}

func Test_InformerWorkQueue_CanListWatch_Allowed(t *testing.T) {
	r := NewInformerWorkQueue(
		zap.New(zap.UseDevMode(true)),
		newAllowingFakeManager(t),
		1, 0, nil, nil, nil,
	)

	if !r.canListWatch(context.Background(), "", "configmaps") {
		t.Errorf("canListWatch(core, \"configmaps\") = false, want true")
	}
	if !r.canListWatch(context.Background(), "datadoghq.com", "datadogagents") {
		t.Errorf("canListWatch(\"datadoghq.com\", \"datadogagents\") = false, want true")
	}
}
