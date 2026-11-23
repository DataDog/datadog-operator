// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagent

import (
	"context"
	"testing"
	"time"

	"github.com/DataDog/dd-trace-go/v2/ddtrace/ext"
	"github.com/DataDog/dd-trace-go/v2/ddtrace/mocktracer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/pkg/testutils"
	"github.com/DataDog/datadog-operator/pkg/trace"
)

func TestReconcileTracing(t *testing.T) {
	mt := mocktracer.Start()
	trace.SetEnabled(true)
	t.Cleanup(func() {
		trace.SetEnabled(false)
		mt.Stop()
	})

	tt := testCase{
		name: "tracing",
		loadFunc: func(c client.Client) *v2alpha1.DatadogAgent {
			dda := testutils.NewInitializedDatadogAgentBuilder("bar", "foo").Build()
			_ = c.Create(context.TODO(), dda)
			return dda
		},
		want: reconcile.Result{RequeueAfter: 15 * time.Second},
	}
	runFullReconcilerTest(t, tt, ReconcilerOptions{})

	roots := map[string]*mocktracer.Span{}
	resources := map[string][]string{}
	spans := mt.FinishedSpans()
	for _, s := range spans {
		if s.ParentID() == 0 {
			roots[s.OperationName()] = s
		}
		resources[s.OperationName()] = append(resources[s.OperationName()], s.Tag(ext.ResourceName).(string))
	}

	ddaRoot := roots[ddaOperationName]
	require.NotNil(t, ddaRoot, "missing DDA root span")
	assert.Equal(t, "Reconcile", ddaRoot.Tag(ext.ResourceName))
	assert.Equal(t, "DatadogAgent", ddaRoot.Tag("kind"))
	assert.Equal(t, "foo", ddaRoot.Tag("name"))
	assert.Equal(t, "bar", ddaRoot.Tag("namespace"))

	ddaiRoot := roots["datadogagentinternal.reconcile"]
	require.NotNil(t, ddaiRoot, "missing DDAI root span")
	assert.Equal(t, "DatadogAgentInternal", ddaiRoot.Tag("kind"))

	assert.Subset(t, resources[ddaOperationName], []string{
		"manageDDADependenciesWithDDAI", "createOrUpdateDDAI", "updateStatusIfNeeded",
	})
	assert.Subset(t, resources["datadogagentinternal.reconcile"], []string{
		"applyAndCleanupDependencies", "reconcileComponent", "reconcileV2Agent",
		"cleanupExtraneousResources", "updateStatusIfNeeded",
	})

	for _, s := range spans {
		root := roots[s.OperationName()]
		require.NotNil(t, root, "span %v has no root", s)
		assert.Equal(t, root.TraceID(), s.TraceID(), "span %v not in root trace", s)
		assert.Equal(t, root.Tag("kind"), s.Tag("kind"))
		assert.Nil(t, s.Tag(ext.ErrorMsg), "unexpected error on span %v", s)
	}
}
