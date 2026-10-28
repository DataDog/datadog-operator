// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagent

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/DataDog/dd-trace-go/v2/ddtrace/mocktracer"
	"github.com/go-logr/logr/funcr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/testutils"
	"github.com/DataDog/datadog-operator/pkg/trace"
)

// Exercise real helper logs with a span-enriched request logger. In particular,
// verbosity-one logs must keep the current span's IDs as well as the flat DDA identity.
func tracedLogContext(t *testing.T) (context.Context, func() []string) {
	t.Helper()
	mt := mocktracer.Start()
	previous := trace.Enabled()
	trace.SetEnabled(true)
	t.Cleanup(func() { trace.SetEnabled(previous); mt.Stop() })
	var logs []string
	logger := funcr.New(func(_, entry string) { logs = append(logs, entry) }, funcr.Options{Verbosity: 1}).WithValues("namespace", "ns", "name", "dda")
	span, ctx := trace.StartSpan(ctrl.LoggerInto(context.Background(), logger))
	require.NotNil(t, span)
	t.Cleanup(func() { trace.FinishSpan(span, nil) })
	return ctx, func() []string {
		t.Helper()
		require.NotEmpty(t, logs)
		spanIDs := map[string]bool{strconv.FormatUint(span.Context().SpanID(), 10): true}
		for _, child := range mt.FinishedSpans() {
			assert.Equal(t, span.Context().TraceIDLower(), child.TraceID())
			spanIDs[strconv.FormatUint(child.SpanID(), 10)] = true
		}
		spanIDPattern := regexp.MustCompile(`"dd.span_id"="([0-9]+)"`)
		for _, entry := range logs {
			assert.Contains(t, entry, `"dd.trace_id"=`)
			ids := spanIDPattern.FindStringSubmatch(entry)
			require.Len(t, ids, 2, entry)
			assert.True(t, spanIDs[ids[1]], "log must reference the request span or one of its children: %s", entry)
			assert.Equal(t, 1, strings.Count(entry, `"namespace"=`), entry)
			assert.Equal(t, 1, strings.Count(entry, `"name"=`), entry)
			assert.Contains(t, entry, `"namespace"="ns"`)
			assert.Contains(t, entry, `"name"="dda"`)
			assert.NotContains(t, entry, `"datadogagent"=`)
		}
		return logs
	}
}

func TestHelpersUseContextLogger(t *testing.T) {
	tests := []struct {
		name    string
		objects []runtime.Object
		run     func(context.Context, *Reconciler) error
	}{
		{
			name:    "unused DDAI cleanup",
			objects: []runtime.Object{&v1alpha1.DatadogAgentInternal{ObjectMeta: metav1.ObjectMeta{Name: "unused", Namespace: "ns"}}},
			run:     func(ctx context.Context, r *Reconciler) error { return r.cleanUpUnusedDDAIs(ctx, nil) },
		},
		{
			name:    "node label changes",
			objects: []runtime.Object{&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node", Labels: map[string]string{"existing": "label"}}}},
			run: func(ctx context.Context, r *Reconciler) error {
				return r.labelNodesWithProfiles(ctx, map[string]types.NamespacedName{"node": {Name: "profile", Namespace: "ns"}})
			},
		},
		{
			name:    "pod profile cleanup",
			objects: []runtime.Object{&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod", Namespace: "ns", Labels: map[string]string{"agent.datadoghq.com/component": constants.DefaultAgentResourceSuffix}}, Spec: corev1.PodSpec{NodeName: "node"}}},
			run: func(ctx context.Context, r *Reconciler) error {
				return r.cleanupPodsForProfilesThatNoLongerApply(ctx, map[string]types.NamespacedName{"node": {Name: "profile", Namespace: "ns"}}, "ns")
			},
		},
		{
			name: "CSI creation",
			run: func(ctx context.Context, r *Reconciler) error {
				return r.createOrUpdateDatadogCSIDriver(ctx, testutils.NewInitializedDatadogAgentBuilder("ns", "dda").BuildWithDefaults())
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, checkLogs := tracedLogContext(t)
			scheme := runtime.NewScheme()
			require.NoError(t, corev1.AddToScheme(scheme))
			require.NoError(t, appsv1.AddToScheme(scheme))
			require.NoError(t, v1alpha1.AddToScheme(scheme))
			require.NoError(t, v2alpha1.AddToScheme(scheme))
			require.NoError(t, storagev1.AddToScheme(scheme))
			r := &Reconciler{scheme: scheme, client: fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(tt.objects...).Build()}
			require.NoError(t, tt.run(ctx, r))
			checkLogs()
		})
	}
}

func TestStatusErrorsUseContextLogger(t *testing.T) {
	t.Run("profile DDAI get error", func(t *testing.T) {
		ctx, checkLogs := tracedLogContext(t)
		r := &Reconciler{client: fake.NewClientBuilder().WithInterceptorFuncs(interceptor.Funcs{
			Get: func(context.Context, client.WithWatch, client.ObjectKey, client.Object, ...client.GetOption) error {
				return errors.New("get failed")
			},
		}).Build()}
		r.addDDAIStatusToProfileStatus(ctx, &v1alpha1.DatadogAgentProfile{}, "dda", "ns", metav1.Now())
		checkLogs()
	})
	for _, conflict := range []bool{false, true} {
		t.Run(fmt.Sprintf("remote config status conflict=%t", conflict), func(t *testing.T) {
			ctx, checkLogs := tracedLogContext(t)
			scheme := runtime.NewScheme()
			require.NoError(t, v1alpha1.AddToScheme(scheme))
			ddai := &v1alpha1.DatadogAgentInternal{ObjectMeta: metav1.ObjectMeta{Name: "dda", Namespace: "ns"}}
			updateErr := errors.New("status update failed")
			if conflict {
				updateErr = apierrors.NewConflict(schema.GroupResource{Group: "datadoghq.com", Resource: "datadogagentinternals"}, ddai.Name, updateErr)
			}
			r := &Reconciler{client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(ddai).WithInterceptorFuncs(interceptor.Funcs{
				SubResourceUpdate: func(context.Context, client.Client, string, client.Object, ...client.SubResourceUpdateOption) error {
					return updateErr
				},
			}).Build()}
			result, err := r.addRemoteConfigStatusToDDAIStatus(ctx, &v2alpha1.DatadogAgentStatus{RemoteConfigConfiguration: &v2alpha1.RemoteConfigConfiguration{}}, ddai.ObjectMeta)
			if conflict {
				require.NoError(t, err)
				assert.Equal(t, time.Second, result.RequeueAfter)
			} else {
				require.ErrorIs(t, err, updateErr)
			}
			checkLogs()
		})
	}
}
