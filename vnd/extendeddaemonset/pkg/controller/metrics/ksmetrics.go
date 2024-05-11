package metrics

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	ksmetric "k8s.io/kube-state-metrics/pkg/metric"
	metricsstore "k8s.io/kube-state-metrics/pkg/metrics_store"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/DataDog/extendeddaemonset/pkg/config"
)

// AddMetrics add given metricFamilies for type given in gvk.
func AddMetrics(gvk schema.GroupVersionKind, mgr manager.Manager, h Handler, metricFamilies []ksmetric.FamilyGenerator) error {
	mapping, err := mgr.GetRESTMapper().RESTMapping(gvk.GroupKind())
	if err != nil {
		return err
	}
	serializerCodec := serializer.NewCodecFactory(mgr.GetScheme())
	paramCodec := runtime.NewParameterCodec(mgr.GetScheme())

	httpClient, err := rest.HTTPClientFor(mgr.GetConfig())
	if err != nil {
		return err
	}

	restClient, err := apiutil.RESTClientForGVK(gvk, false, mgr.GetConfig(), serializerCodec, httpClient)
	if err != nil {
		return err
	}

	obj, err := mgr.GetScheme().New(gvk)
	if err != nil {
		return err
	}

	listGVK := gvk.GroupVersion().WithKind(gvk.Kind + "List")
	listObj, err := mgr.GetScheme().New(listGVK)
	if err != nil {
		return err
	}

	namespaces := config.GetWatchNamespaces()
	if len(namespaces) == 0 {
		namespaces = append(namespaces, "")
	}
	for _, ns := range namespaces {
		lw := &cache.ListWatch{
			ListFunc: func(opts metav1.ListOptions) (runtime.Object, error) {
				res := listObj.DeepCopyObject()
				localErr := restClient.Get().NamespaceIfScoped(ns, ns != "").Resource(mapping.Resource.Resource).VersionedParams(&opts, paramCodec).Do(context.Background()).Into(res)

				return res, localErr
			},
			WatchFunc: func(opts metav1.ListOptions) (watch.Interface, error) {
				opts.Watch = true

				return restClient.Get().NamespaceIfScoped(ns, ns != "").Resource(mapping.Resource.Resource).VersionedParams(&opts, paramCodec).Watch(context.Background())
			},
		}

		err = h.RegisterStore(metricFamilies, obj, lw)
		if err != nil {
			return err
		}
	}

	return nil
}

// newMetricsStore return new metrics store.
func newMetricsStore(generators []ksmetric.FamilyGenerator, expectedType interface{}, lw cache.ListerWatcher) *metricsstore.MetricsStore {
	// Generate collector per namespace.
	composedMetricGenFuncs := ksmetric.ComposeMetricGenFuncs(generators)
	headers := ksmetric.ExtractMetricFamilyHeaders(generators)
	store := metricsstore.NewMetricsStore(headers, composedMetricGenFuncs)
	reflectorPerNamespace(context.TODO(), lw, expectedType, store)

	return store
}

func reflectorPerNamespace(
	ctx context.Context,
	lw cache.ListerWatcher,
	expectedType interface{},
	store cache.Store,
) {
	reflector := cache.NewReflector(lw, expectedType, store, 0)
	go reflector.Run(ctx.Done())
}
