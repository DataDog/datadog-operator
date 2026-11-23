// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flare

import (
	"context"
	"fmt"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// pagedReader serves total objects in pages of the requested limit, and
// records the page sizes it was asked for.
type pagedReader struct {
	client.Reader
	total  int
	name   func(i int) string
	limits []int64
}

func (r *pagedReader) List(_ context.Context, list client.ObjectList, opts ...client.ListOption) error {
	listOpts := (&client.ListOptions{}).ApplyOptions(opts)
	r.limits = append(r.limits, listOpts.Limit)
	start := 0
	if listOpts.Continue != "" {
		start, _ = strconv.Atoi(listOpts.Continue)
	}
	end := min(start+int(listOpts.Limit), r.total)
	next := ""
	if end < r.total {
		next = strconv.Itoa(end)
	}
	switch l := list.(type) {
	case *unstructured.UnstructuredList:
		for i := start; i < end; i++ {
			item := unstructured.Unstructured{}
			item.SetName(r.name(i))
			l.Items = append(l.Items, item)
		}
		l.SetContinue(next)
	case *metav1.PartialObjectMetadataList:
		for i := start; i < end; i++ {
			l.Items = append(l.Items, metav1.PartialObjectMetadata{ObjectMeta: metav1.ObjectMeta{Name: r.name(i)}})
		}
		l.Continue = next
	default:
		return fmt.Errorf("unexpected list %T", list)
	}
	return nil
}

func TestEachUnstructuredPage(t *testing.T) {
	reader := &pagedReader{total: maxListItems + 50, name: func(i int) string { return strconv.Itoa(i) }}
	var sizes []int
	found, truncated, err := eachUnstructuredPage(context.Background(), reader, datadogAgentGVK, func(page []unstructured.Unstructured) bool {
		sizes = append(sizes, len(page))
		return true
	})
	require.NoError(t, err)
	assert.True(t, found)
	assert.True(t, truncated)
	assert.Len(t, sizes, maxListItems/listPageSize, "pages stop at maxListItems")
	for _, size := range sizes {
		assert.Equal(t, listPageSize, size)
	}

	pages := 0
	_, truncated, err = eachUnstructuredPage(context.Background(), reader, datadogAgentGVK, func([]unstructured.Unstructured) bool {
		pages++
		return false
	})
	require.NoError(t, err)
	assert.True(t, truncated)
	assert.Equal(t, 1, pages, "listing stops when the page callback returns false")
}

func TestDatadogCRDNames(t *testing.T) {
	reader := &pagedReader{total: 1000, name: func(i int) string {
		if i%100 == 0 {
			return fmt.Sprintf("kind%d.datadoghq.com", i)
		}
		return fmt.Sprintf("kind%d.example.com", i)
	}}
	names, err := datadogCRDNames(context.Background(), reader)
	require.NoError(t, err)
	assert.Len(t, names, 10)
	assert.Contains(t, names, "kind900.datadoghq.com")
	for _, limit := range reader.limits {
		assert.Equal(t, int64(crdMetadataPageSize), limit, "CRDs are listed in small metadata pages")
	}
}
