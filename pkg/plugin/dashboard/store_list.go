// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package dashboard

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"

	"github.com/DataDog/datadog-operator/pkg/plugin/helm"
)

// HelmHistoryFunc reads the history of a Helm release, newest first.
type HelmHistoryFunc func(ctx context.Context, namespace, release string) ([]helm.Revision, error)

// ListClients are the clients a ListStore reads with. They are only used
// with the get and list verbs.
type ListClients struct {
	Dynamic dynamic.Interface
	// Kube serves discovery and pods.
	Kube kubernetes.Interface
	// HelmHistory reads Helm release histories; nil disables the lookup
	// (--no-helm).
	HelmHistory HelmHistoryFunc
}

// ListConfig configures a ListStore.
type ListConfig struct {
	// Namespace is where the DDA is looked for; ignored with AllNamespaces.
	Namespace     string
	AllNamespaces bool
	// DDAName selects the DDA by name; empty means the only one in scope.
	DDAName string
	// Pods lists the agent pods (--pods).
	Pods bool
	// Cluster holds the context and plugin version; the server version is
	// read from discovery.
	Cluster ClusterInfo
	// Now is the clock; it defaults to time.Now.
	Now func() time.Time
}

// ListStore is the one-shot Store of the static mode. Start reads
// discovery, then the DDAs, then every other source in parallel. A source
// that can't be read records its state without aborting the others.
type ListStore struct {
	clients ListClients
	cfg     ListConfig

	mu   sync.Mutex
	snap *Snapshot
}

var _ Store = &ListStore{}

// NewListStore returns a ListStore.
func NewListStore(clients ListClients, cfg ListConfig) *ListStore {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &ListStore{clients: clients, cfg: cfg}
}

// helmHistoryLimit is the number of release revisions read; only the two
// newest are used.
const helmHistoryLimit = 2

// Start implements Store. It returns ErrUnsupportedOperator when a required
// CRD is not served, and an error when discovery fails or ctx is done. Any
// other failure is recorded in the snapshot sources.
func (s *ListStore) Start(ctx context.Context) error {
	now := s.cfg.Now()
	s.snap = &Snapshot{
		Taken:   now,
		Cluster: s.cfg.Cluster,
		Objects: map[string][]*unstructured.Unstructured{},
		Helm:    map[string][]helm.Revision{},
		Sources: map[string]SourceInfo{},
	}
	sources := Sources()
	served, version, err := discover(ctx, s.clients.Kube, sources)
	if err != nil {
		return err
	}
	s.snap.Cluster.ServerVersion = version
	byKey := map[string]Source{}
	for _, src := range sources {
		byKey[src.Key] = src
		if !served[src.GVR] {
			s.record(src.Key, SourceInfo{State: SourceNotInstalled}, nil)
		}
	}
	if unsupported(sources, served) {
		return ErrUnsupportedOperator
	}

	// The DDA selects the namespace and label selector of every other
	// source. Build reports a missing or ambiguous DDA.
	r := reader{dynamic: s.clients.Dynamic, kube: s.clients.Kube}
	info, objs := r.readList(ctx, byKey[SourceDDA], s.cfg.ddaNamespace(), "")
	s.record(SourceDDA, info, objs)
	if err = ctx.Err(); err != nil {
		return err
	}
	if info.State != SourceOK {
		return nil
	}
	dda, selErr := selectDDA(objs, s.cfg.DDAName)
	if selErr != nil {
		return nil //nolint:nilerr // Build reports a missing or ambiguous DDA.
	}
	sel := scope{namespace: dda.GetNamespace(), dda: dda.GetName()}

	g, gctx := errgroup.WithContext(ctx)
	for _, src := range sources {
		if !served[src.GVR] {
			continue
		}
		switch src.Key {
		case SourceDDA, SourceLease:
			continue // read before and after this wave
		case SourcePods:
			if !s.cfg.Pods {
				continue
			}
			g.Go(func() error {
				info, pods := r.readPods(gctx, src, sel.namespace, sel.selector(src))
				s.mu.Lock()
				s.snap.Pods = pods
				s.mu.Unlock()
				s.record(src.Key, info, nil)
				return gctx.Err()
			})
		default:
			g.Go(func() error {
				info, objs := r.read(gctx, src, sel)
				s.record(src.Key, info, objs)
				return gctx.Err()
			})
		}
	}
	if err := g.Wait(); err != nil {
		return err
	}

	// Both depend on objects read above, before the goroutines below write
	// to the snapshot.
	op := pickOperator(s.snap.Objects[SourceOperator], sel.namespace)
	releases := helmReleases(dda, s.snap.Objects[SourceDAP])
	g, gctx = errgroup.WithContext(ctx)
	if src := byKey[SourceLease]; served[src.GVR] && op != nil {
		g.Go(func() error {
			info, objs := r.readNamed(gctx, src, op.GetNamespace())
			s.record(src.Key, info, objs)
			return gctx.Err()
		})
	}
	if s.clients.HelmHistory != nil {
		for _, rel := range releases {
			g.Go(func() error {
				if revs, ok := readHelm(gctx, s.clients.HelmHistory, rel); ok {
					s.mu.Lock()
					s.snap.Helm[rel.Key()] = revs
					s.mu.Unlock()
				}
				return gctx.Err()
			})
		}
	}
	return g.Wait()
}

// ddaNamespace is the namespace the DDAs are listed in; "" is all
// namespaces.
func (c ListConfig) ddaNamespace() string {
	if c.AllNamespaces {
		return metav1.NamespaceAll
	}
	return c.Namespace
}

// Snapshot implements Store.
func (s *ListStore) Snapshot() *Snapshot { return s.snap }

// Changed implements Store. A ListStore never changes.
func (s *ListStore) Changed() <-chan struct{} { return nil }

// Close implements Store.
func (s *ListStore) Close() {}

// record stores the objects and state of a source.
func (s *ListStore) record(key string, info SourceInfo, objs []*unstructured.Unstructured) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if info.Mode == "" {
		info.Mode = RefreshOnce
	}
	if info.State == SourceOK {
		info.LastOK = s.snap.Taken
		s.snap.Objects[key] = objs
	}
	s.snap.Sources[key] = info
}

// scope is where the sources of the selected DDA are listed.
type scope struct {
	namespace, dda string
}

func (sc scope) selector(src Source) string {
	if src.Selector == nil {
		return ""
	}
	return src.Selector(sc.dda)
}

// discover returns the resources served among the sources and the server
// version. Group versions that are not served at all are not an error.
func discover(ctx context.Context, kube kubernetes.Interface, sources []Source) (map[schema.GroupVersionResource]bool, string, error) {
	disco := kube.Discovery()
	var gvs []schema.GroupVersion
	for _, src := range sources {
		if gv := src.GVR.GroupVersion(); !slices.Contains(gvs, gv) {
			gvs = append(gvs, gv)
		}
	}

	var mu sync.Mutex
	served := map[schema.GroupVersionResource]bool{}
	var version string
	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		info, err := disco.ServerVersion()
		if err != nil {
			return fmt.Errorf("cannot read the server version: %w", err)
		}
		version = info.GitVersion
		return gctx.Err()
	})
	for _, gv := range gvs {
		g.Go(func() error {
			list, err := disco.ServerResourcesForGroupVersion(gv.String())
			if apierrors.IsNotFound(err) {
				return gctx.Err()
			}
			if err != nil {
				return fmt.Errorf("cannot discover %s: %w", gv, err)
			}
			mu.Lock()
			defer mu.Unlock()
			for _, r := range list.APIResources {
				served[gv.WithResource(r.Name)] = true
			}
			return gctx.Err()
		})
	}
	if err := g.Wait(); err != nil {
		return nil, "", err
	}
	return served, version, nil
}

// unsupported reports whether a required source is not served.
func unsupported(sources []Source, served map[schema.GroupVersionResource]bool) bool {
	for _, src := range sources {
		if src.Required && !served[src.GVR] {
			return true
		}
	}
	return false
}

// failed returns the source info of a failed read.
func failed(err error) SourceInfo {
	return SourceInfo{State: classify(err), Err: err.Error()}
}

// classify maps a read error to a source state.
func classify(err error) SourceState {
	switch {
	case apierrors.IsForbidden(err), apierrors.IsUnauthorized(err):
		return SourceForbidden
	case apierrors.IsNotFound(err):
		// The collection itself is not served.
		return SourceNotInstalled
	default:
		return SourceError
	}
}

// reader reads sources with the get and list verbs. Its methods return the
// state and objects of a source; both stores share them.
type reader struct {
	dynamic dynamic.Interface
	kube    kubernetes.Interface
}

// read reads a source of the selected DDA, other than pods and the Lease.
func (r reader) read(ctx context.Context, src Source, sel scope) (SourceInfo, []*unstructured.Unstructured) {
	switch {
	case src.Key == SourceOperator:
		return r.readOperator(ctx, src, sel)
	case src.Scope == ScopeClusterFallback:
		return r.readClusterFallback(ctx, src, sel)
	default:
		return r.readList(ctx, src, sel.namespace, sel.selector(src))
	}
}

// list lists a source in namespace ("" is all namespaces) and applies its
// transform.
func (r reader) list(ctx context.Context, src Source, namespace, selector string) ([]*unstructured.Unstructured, error) {
	list, err := r.dynamic.Resource(src.GVR).Namespace(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return nil, err
	}
	out := make([]*unstructured.Unstructured, 0, len(list.Items))
	for i := range list.Items {
		obj, err := src.Transform(&list.Items[i])
		if err != nil {
			return nil, err
		}
		out = append(out, obj.(*unstructured.Unstructured))
	}
	return out, nil
}

// readList lists a source in one namespace.
func (r reader) readList(ctx context.Context, src Source, namespace, selector string) (SourceInfo, []*unstructured.Unstructured) {
	objs, err := r.list(ctx, src, namespace, selector)
	if err != nil {
		return failed(err), nil
	}
	return SourceInfo{State: SourceOK}, objs
}

// readClusterFallback lists a source in all namespaces, falling back to the
// DDA namespace with a notice when that is forbidden.
func (r reader) readClusterFallback(ctx context.Context, src Source, sel scope) (SourceInfo, []*unstructured.Unstructured) {
	objs, err := r.list(ctx, src, metav1.NamespaceAll, sel.selector(src))
	if err == nil {
		return SourceInfo{State: SourceOK}, objs
	}
	if classify(err) != SourceForbidden {
		return failed(err), nil
	}
	objs, err = r.list(ctx, src, sel.namespace, sel.selector(src))
	if err != nil {
		return failed(err), nil
	}
	return SourceInfo{State: SourceOK, Notice: fallbackNotice(src, sel.namespace)}, objs
}

// fallbackNotice is the notice of a source listed in the DDA namespace only.
// Summary kinds share one notice, which Build shows once.
func fallbackNotice(src Source, namespace string) string {
	what := src.Kind + "s"
	if IsSummarySource(src.Key) {
		what = "Other resources"
	}
	return fmt.Sprintf("%s: namespace %s only (cluster-wide list forbidden)", what, namespace)
}

// readOperator looks for the operator Deployment in the DDA namespace, then
// in all namespaces.
func (r reader) readOperator(ctx context.Context, src Source, sel scope) (SourceInfo, []*unstructured.Unstructured) {
	selector := sel.selector(src)
	local, localErr := r.list(ctx, src, sel.namespace, selector)
	if localErr == nil && len(local) > 0 {
		return SourceInfo{State: SourceOK}, local
	}
	all, err := r.list(ctx, src, metav1.NamespaceAll, selector)
	switch {
	case err == nil:
		return SourceInfo{State: SourceOK}, all
	case localErr == nil:
		return SourceInfo{State: SourceOK, Notice: fmt.Sprintf("operator: not found in namespace %s, and other namespaces can't be listed", sel.namespace)}, local
	default:
		return failed(err), nil
	}
}

// readNamed reads the one object of a named source; a missing object is not
// an error.
func (r reader) readNamed(ctx context.Context, src Source, namespace string) (SourceInfo, []*unstructured.Unstructured) {
	obj, err := r.dynamic.Resource(src.GVR).Namespace(namespace).Get(ctx, src.Name, metav1.GetOptions{})
	switch {
	case apierrors.IsNotFound(err):
		return SourceInfo{State: SourceOK}, nil
	case err != nil:
		return failed(err), nil
	}
	stripped, err := src.Transform(obj)
	if err != nil {
		return failed(err), nil
	}
	return SourceInfo{State: SourceOK}, []*unstructured.Unstructured{stripped.(*unstructured.Unstructured)}
}

// readPods lists the agent pods of the DDA with the typed client, stripped
// to the fields the dashboard uses.
func (r reader) readPods(ctx context.Context, src Source, namespace, selector string) (SourceInfo, []*corev1.Pod) {
	list, err := r.kube.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return failed(err), nil
	}
	pods := make([]*corev1.Pod, 0, len(list.Items))
	for i := range list.Items {
		stripped, err := src.Transform(&list.Items[i])
		if err != nil {
			return failed(err), nil
		}
		pods = append(pods, stripped.(*corev1.Pod))
	}
	return SourceInfo{State: SourceOK}, pods
}

// readHelm reads the history of a release, keeping the newest revisions.
// Failures leave the history out, so that the chart label is shown instead.
func readHelm(ctx context.Context, history HelmHistoryFunc, rel ReleaseRef) ([]helm.Revision, bool) {
	revs, err := history(ctx, rel.Namespace, rel.Name)
	if err != nil || len(revs) == 0 {
		return nil, false
	}
	if len(revs) > helmHistoryLimit {
		revs = revs[:helmHistoryLimit]
	}
	return revs, true
}

// helmReleases returns the distinct Helm releases of the DDA and the DAPs.
func helmReleases(dda *unstructured.Unstructured, daps []*unstructured.Unstructured) []ReleaseRef {
	var out []ReleaseRef
	for _, obj := range append([]*unstructured.Unstructured{dda}, daps...) {
		if rel, ok := HelmRelease(obj); ok && !slices.Contains(out, rel) {
			out = append(out, rel)
		}
	}
	return out
}
