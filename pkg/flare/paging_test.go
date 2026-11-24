// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flare

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/DataDog/datadog-operator/pkg/flare/flaretest"
)

// pagedReader serves total objects in pages of the requested limit, and
// records the page sizes it was asked for.
type pagedReader struct {
	client.Reader
	total     int
	name      func(i int) string
	limits    []int64
	remaining *int64
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
		if start == 0 && next != "" {
			l.RemainingItemCount = r.remaining
		}
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

func TestSampledResourceCollector(t *testing.T) {
	reader := &pagedReader{total: 340, name: func(i int) string { return fmt.Sprintf("profile-%03d", i) }}
	files, manifest := runCollectors(t, Env{Reader: reader}, sampledResourceCollector("datadog/datadogagentprofiles", datadogAgentProfileGVK))

	require.Len(t, manifest.Collectors, 1)
	assert.Equal(t, []string{"collected the first 100 of 340 DatadogAgentProfile"}, manifest.Collectors[0].Errors)
	sampled := 0
	for name := range files {
		if strings.HasPrefix(name, "datadog/datadogagentprofiles/") {
			sampled++
		}
	}
	assert.Equal(t, maxSampledResources, sampled)
	assert.Equal(t, int64(maxSampledResources), reader.limits[0], "the API server is asked for the sample only")
}

func TestCountResources(t *testing.T) {
	reader := &pagedReader{total: 1234, name: strconv.Itoa}
	count, err := countResources(context.Background(), reader, datadogAgentGVK)
	require.NoError(t, err)
	assert.Equal(t, 1234, count)

	remaining := int64(5000)
	count, err = countResources(context.Background(), &pagedReader{total: 60, name: strconv.Itoa, remaining: &remaining}, datadogAgentGVK)
	require.NoError(t, err)
	assert.Equal(t, 5050, count, "the remaining item count of the first page is used")
}

// listErrReader fails List for unstructured or metadata-only lists.
type listErrReader struct {
	client.Reader
	unstructuredErr, metadataErr error
}

func (r listErrReader) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	switch list.(type) {
	case *unstructured.UnstructuredList:
		if r.unstructuredErr != nil {
			return r.unstructuredErr
		}
	case *metav1.PartialObjectMetadataList:
		if r.metadataErr != nil {
			return r.metadataErr
		}
	}
	return r.Reader.List(ctx, list, opts...)
}

func TestSampledResourceCollectorErrors(t *testing.T) {
	notInstalled := &meta.NoKindMatchError{GroupKind: datadogAgentProfileGVK.GroupKind()}
	files, manifest := runCollectors(t, Env{Reader: listErrReader{unstructuredErr: notInstalled}}, sampledResourceCollector("datadog/datadogagentprofiles", datadogAgentProfileGVK))
	assert.Equal(t, StatusOK, manifest.Collectors[0].Status)
	assert.Contains(t, files, "datadog/datadogagentprofiles/NOT_INSTALLED.txt")

	_, manifest = runCollectors(t, Env{Reader: listErrReader{unstructuredErr: errors.New("forbidden")}}, sampledResourceCollector("datadog/datadogagentprofiles", datadogAgentProfileGVK))
	assert.Equal(t, StatusError, manifest.Collectors[0].Status)

	// More objects than the cap, and the count fails: the sample is still
	// collected, and the manifest says the total is unknown.
	reader := listErrReader{Reader: &pagedReader{total: maxSampledResources + 1, name: strconv.Itoa}, metadataErr: errors.New("timeout")}
	files, manifest = runCollectors(t, Env{Reader: reader}, sampledResourceCollector("datadog/datadogagentprofiles", datadogAgentProfileGVK))
	assert.Equal(t, StatusPartial, manifest.Collectors[0].Status)
	assert.Equal(t, []string{"collected the first 100 DatadogAgentProfile, unable to count the others: timeout"}, manifest.Collectors[0].Errors)
	assert.Contains(t, files, "datadog/datadogagentprofiles/0.yaml")
}

func TestCollectResourceCounts(t *testing.T) {
	files, manifest := runCollectors(t, Env{Reader: listErrReader{metadataErr: &meta.NoKindMatchError{}}}, Collector{Name: "counts", Collect: collectResourceCounts})
	assert.Equal(t, StatusOK, manifest.Collectors[0].Status)
	assert.Contains(t, files["datadog/resources-count.yaml"], "DatadogMonitor: not installed")

	files, manifest = runCollectors(t, Env{Reader: listErrReader{metadataErr: errors.New("forbidden")}}, Collector{Name: "counts", Collect: collectResourceCounts})
	assert.Equal(t, StatusPartial, manifest.Collectors[0].Status)
	assert.Contains(t, files["datadog/resources-count.yaml"], "DatadogSLO: 'error: forbidden'")
}

// crdGetErrReader fails Get for one CRD.
type crdGetErrReader struct {
	client.Reader
	failName string
}

func (r crdGetErrReader) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if key.Name == r.failName {
		return errors.New("forbidden")
	}
	return r.Reader.Get(ctx, key, obj, opts...)
}

func TestCollectCRDsGetError(t *testing.T) {
	crd := func(name string) *apiextensionsv1.CustomResourceDefinition {
		return &apiextensionsv1.CustomResourceDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: name},
			Spec:       apiextensionsv1.CustomResourceDefinitionSpec{Group: datadogGroup},
		}
	}
	reader, _ := flaretest.Clients(t, crd("datadogagents.datadoghq.com"), crd("datadogmonitors.datadoghq.com"))
	files, manifest := runCollectors(t, Env{Reader: crdGetErrReader{Reader: reader, failName: "datadogmonitors.datadoghq.com"}}, Collector{Name: "crds", Collect: collectCRDs})

	assert.Equal(t, StatusPartial, manifest.Collectors[0].Status)
	assert.Equal(t, []string{"unable to get CustomResourceDefinition datadogmonitors.datadoghq.com: forbidden"}, manifest.Collectors[0].Errors)
	assert.Contains(t, files["config/crds.yaml"], "datadogagents.datadoghq.com", "the other CRDs are still collected")
}
