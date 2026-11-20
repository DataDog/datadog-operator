// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flare

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubefake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

// fakeLog serves a log of n lines: the last TailLines lines when the request
// has TailLines, the whole log otherwise, like the kubelet.
func fakeLog(t *testing.T, env Env, n int, requests *[]corev1.PodLogOptions) {
	t.Helper()
	lines := make([]string, n)
	for i := range lines {
		lines[i] = fmt.Sprintf("line %d", i)
	}
	env.Clientset.(*kubefake.Clientset).PrependReactor("get", "pods", func(action k8stesting.Action) (bool, runtime.Object, error) {
		if action.GetSubresource() != "log" {
			return false, nil, nil
		}
		opts := *action.(k8stesting.GenericAction).GetValue().(*corev1.PodLogOptions)
		*requests = append(*requests, opts)
		served := lines
		if opts.TailLines != nil && int(*opts.TailLines) < len(lines) {
			served = lines[len(lines)-int(*opts.TailLines):]
		}
		return true, &runtime.Unknown{Raw: []byte(strings.Join(served, "\n") + "\n")}, nil
	})
}

func TestCollectPodLogsHead(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "op", Namespace: "datadog"},
		Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "manager"}}},
		Status:     corev1.PodStatus{ContainerStatuses: []corev1.ContainerStatus{{Name: "manager", RestartCount: 1}}},
	}
	collect := Collector{Name: "logs", Collect: func(ctx context.Context, c *Collection) error {
		CollectPodLogs(ctx, c, "logs", pod)
		return nil
	}}

	t.Run("long log: tail and head of the current and previous containers", func(t *testing.T) {
		env := testEnv(t, pod)
		env.LogTailLines, env.LogHeadLines = 10, 3
		var requests []corev1.PodLogOptions
		fakeLog(t, env, 100, &requests)

		files, manifest := runCollectors(t, env, collect)
		assert.Equal(t, StatusOK, manifest.Collectors[0].Status, manifest.Collectors[0].Errors)
		assert.Equal(t, "line 0\nline 1\nline 2\n", files["logs/op/manager.head.log"])
		assert.Equal(t, "line 0\nline 1\nline 2\n", files["logs/op/manager.previous.head.log"])
		assert.True(t, strings.HasPrefix(files["logs/op/manager.log"], "line 90\n"))
		assert.True(t, strings.HasSuffix(files["logs/op/manager.log"], "line 99\n"))

		require.Len(t, requests, 4)
		for _, opts := range requests[1:2] {
			assert.Nil(t, opts.TailLines, "the head is read from the start of the log")
			require.NotNil(t, opts.LimitBytes)
			assert.LessOrEqual(t, *opts.LimitBytes, DefaultLogHeadBytes)
			assert.False(t, opts.Previous)
		}
		assert.True(t, requests[3].Previous)
		assert.Nil(t, requests[3].TailLines)
	})

	t.Run("short log: the tail holds the whole log, no head", func(t *testing.T) {
		env := testEnv(t, pod)
		env.LogTailLines = 10
		var requests []corev1.PodLogOptions
		fakeLog(t, env, 5, &requests)

		files, manifest := runCollectors(t, env, collect)
		assert.Equal(t, StatusOK, manifest.Collectors[0].Status)
		assert.Contains(t, files, "logs/op/manager.log")
		assert.NotContains(t, files, "logs/op/manager.head.log")
		assert.NotContains(t, files, "logs/op/manager.previous.head.log")
		assert.Len(t, requests, 2)
	})

	t.Run("tail cut by the byte limit: head collected", func(t *testing.T) {
		env := testEnv(t, pod)
		env.LogLimitBytes, env.LogHeadBytes = 20, 14
		var requests []corev1.PodLogOptions
		fakeLog(t, env, 100, &requests)

		files, _ := runCollectors(t, env, collect)
		assert.Len(t, files["logs/op/manager.log"], 20)
		// The head stops at the byte limit, in the middle of a line.
		assert.Equal(t, "line 0\nline 1\n", files["logs/op/manager.head.log"])
	})

	t.Run("no room left for the head", func(t *testing.T) {
		env := testEnv(t, pod)
		env.LogTailLines = 10
		var requests []corev1.PodLogOptions
		fakeLog(t, env, 100, &requests)
		archive := NewArchive(ArchiveOptions{MaxBytes: manifestReserve + 70})
		manifest := Run(context.Background(), env, archive, []Collector{collect}, RunOptions{})
		assert.Equal(t, StatusPartial, manifest.Collectors[0].Status)
		assert.Contains(t, strings.Join(manifest.Collectors[0].Errors, "\n"), "size limit")
	})
}
