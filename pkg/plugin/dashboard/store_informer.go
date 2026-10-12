// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package dashboard

import (
	"cmp"
	"context"
	"errors"
	"io"
	"maps"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/cache"

	"github.com/DataDog/datadog-operator/pkg/plugin/helm"
)

// Refresh intervals of the InformerStore.
const (
	// DefaultPollInterval refreshes the summary kinds, the operator and its
	// Lease.
	DefaultPollInterval = 30 * time.Second
	// DefaultFallbackInterval refreshes a source whose watch is forbidden
	// while its list is allowed.
	DefaultFallbackInterval = 15 * time.Second
	// DefaultHelmInterval refreshes the Helm histories while a rollout is
	// in progress.
	DefaultHelmInterval = 60 * time.Second
)

// InformerConfig configures an InformerStore.
type InformerConfig struct {
	ListConfig
	// PollInterval defaults to DefaultPollInterval.
	PollInterval time.Duration
	// FallbackInterval defaults to DefaultFallbackInterval.
	FallbackInterval time.Duration
	// HelmInterval defaults to DefaultHelmInterval.
	HelmInterval time.Duration
	// Go starts the store's goroutines, e.g. with a panic handler. It
	// defaults to the go statement.
	Go func(func())
}

// InformerStore is the live Store. It reads the same source table as the
// ListStore: sources with Mode Watch are watched with informers, the others
// are polled. The DDA is watched first; the other sources are scoped by the
// selected DDA and restarted when the selection changes. Every source keeps
// its own state: Loading until synced, Forbidden, NotInstalled, Error, or
// Disconnected while its watch fails. A source whose watch is forbidden
// while its list is allowed is polled instead. The clients are only used
// with the get, list and watch verbs.
type InformerStore struct {
	clients ListClients
	reader  reader
	cfg     InformerConfig

	changed    chan struct{}
	selectKick chan struct{}
	helmKick   atomic.Pointer[chan struct{}]
	version    atomic.Uint64

	ctx       context.Context //nolint:containedctx // bounds the store's goroutines
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	closeOnce sync.Once

	mu        sync.Mutex
	cluster   ClusterInfo
	sources   map[string]*liveSource
	helm      map[string][]helm.Revision
	sel       scope
	depCancel context.CancelFunc
	cached    *Snapshot
	cachedVer uint64
}

var _ Store = &InformerStore{}

// liveSource is the state of one source and its current run.
type liveSource struct {
	src  Source
	info SourceInfo
	run  *sourceRun
}

// sourceRun is one way of acquiring a source: an informer, or a poller
// filling objs and pods. A run that is no longer its source's current run
// is ignored.
type sourceRun struct {
	s  *InformerStore
	ls *liveSource
	// parent is the context a replacement run is started from; ctx stops
	// this run. Callbacks from the informer read them.
	parent context.Context //nolint:containedctx // see above
	ctx    context.Context //nolint:containedctx // see above
	cancel context.CancelFunc

	namespace, selector, notice string

	informer cache.SharedIndexInformer
	// objs and pods are the polled objects, guarded by InformerStore.mu.
	objs []*unstructured.Unstructured
	pods []*corev1.Pod

	lastEvent atomic.Int64
	resolving atomic.Bool
}

// NewInformerStore returns an InformerStore.
func NewInformerStore(clients ListClients, cfg InformerConfig) *InformerStore {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	cfg.PollInterval = cmp.Or(cfg.PollInterval, DefaultPollInterval)
	cfg.FallbackInterval = cmp.Or(cfg.FallbackInterval, DefaultFallbackInterval)
	cfg.HelmInterval = cmp.Or(cfg.HelmInterval, DefaultHelmInterval)
	return &InformerStore{
		clients:    clients,
		reader:     reader{dynamic: clients.Dynamic, kube: clients.Kube},
		cfg:        cfg,
		changed:    make(chan struct{}, 1),
		selectKick: make(chan struct{}, 1),
		cluster:    cfg.Cluster,
		sources:    map[string]*liveSource{},
		helm:       map[string][]helm.Revision{},
	}
}

// Start implements Store. It reads discovery, starts the DDA watch and
// returns; every source is Loading until it is read. It returns
// ErrUnsupportedOperator when a required CRD is not served, and an error
// when discovery fails. ctx bounds the lifetime of the store.
func (s *InformerStore) Start(ctx context.Context) error {
	sources := Sources()
	served, version, err := discover(ctx, s.clients.Kube, sources)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cluster.ServerVersion = version
	for _, src := range sources {
		if !served[src.GVR] {
			s.sources[src.Key] = &liveSource{src: src, info: SourceInfo{State: SourceNotInstalled, Mode: src.Mode}}
		}
	}
	s.notify()
	if unsupported(sources, served) {
		return ErrUnsupportedOperator
	}
	s.ctx, s.cancel = context.WithCancel(ctx)
	for _, src := range sources {
		if src.Key == SourceDDA {
			s.startWatch(s.ctx, src, s.cfg.ddaNamespace(), "", "")
		}
	}
	s.goFunc(s.selectLoop)
	return nil
}

// Snapshot implements Store. Snapshots are cached until the data changes.
// Their objects are shared with the store and must not be modified.
func (s *InformerStore) Snapshot() *Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	ver := s.version.Load()
	if s.cached != nil && s.cachedVer == ver {
		return s.cached
	}
	snap := &Snapshot{
		Taken:   s.cfg.Now(),
		Cluster: s.cluster,
		Objects: map[string][]*unstructured.Unstructured{},
		Helm:    maps.Clone(s.helm),
		Sources: make(map[string]SourceInfo, len(s.sources)),
	}
	for key, ls := range s.sources {
		info := ls.info
		if r := ls.run; r != nil {
			if t := r.lastEvent.Load(); t > 0 && (info.State == SourceOK || info.State == SourceDisconnected) {
				if last := time.Unix(0, t); last.After(info.LastOK) {
					info.LastOK = last
				}
			}
			if key == SourcePods {
				snap.Pods = r.podList()
			} else {
				snap.Objects[key] = r.objectList()
			}
		}
		snap.Sources[key] = info
	}
	s.cached, s.cachedVer = snap, ver
	return snap
}

// Changed implements Store. The channel has a capacity of one, so a burst
// of changes wakes the reader once.
func (s *InformerStore) Changed() <-chan struct{} { return s.changed }

// Close implements Store. It stops every informer and poller and waits for
// them.
func (s *InformerStore) Close() {
	s.closeOnce.Do(func() {
		s.mu.Lock()
		cancel := s.cancel
		s.mu.Unlock()
		if cancel != nil {
			cancel()
		}
		s.wg.Wait()
	})
}

func (s *InformerStore) goFunc(f func()) {
	if s.cfg.Go == nil {
		s.wg.Go(f)
		return
	}
	s.wg.Add(1)
	s.cfg.Go(func() {
		defer s.wg.Done()
		f()
	})
}

// notify records a change and signals Changed without blocking.
func (s *InformerStore) notify() {
	s.version.Add(1)
	select {
	case s.changed <- struct{}{}:
	default:
	}
}

func kick(ch chan struct{}) {
	select {
	case ch <- struct{}{}:
	default:
	}
}

func (s *InformerStore) kickHelm() {
	if ch := s.helmKick.Load(); ch != nil {
		kick(*ch)
	}
}

// kickDependents wakes the loops that depend on a source's objects.
func (s *InformerStore) kickDependents(key string) {
	switch key {
	case SourceDDA:
		kick(s.selectKick)
		s.kickHelm()
	case SourceDAP:
		s.kickHelm()
	}
}

// current reports whether r is the current run of its source. mu is held.
func (s *InformerStore) current(r *sourceRun) bool {
	return s.sources[r.ls.src.Key] == r.ls && r.ls.run == r
}

// sourceLocked returns the live source of src, creating it. mu is held.
func (s *InformerStore) sourceLocked(src Source) *liveSource {
	ls, ok := s.sources[src.Key]
	if !ok {
		ls = &liveSource{src: src}
		s.sources[src.Key] = ls
	}
	return ls
}

// newRun replaces the current run of src. mu is held.
func (s *InformerStore) newRun(parent context.Context, src Source, namespace, selector, notice string, mode RefreshMode) *sourceRun {
	ls := s.sourceLocked(src)
	if ls.run != nil {
		ls.run.cancel()
	}
	ctx, cancel := context.WithCancel(parent)
	r := &sourceRun{s: s, ls: ls, parent: parent, ctx: ctx, cancel: cancel, namespace: namespace, selector: selector, notice: notice}
	ls.run = r
	ls.info = SourceInfo{State: SourceLoading, Mode: mode, Notice: notice}
	s.notify()
	return r
}

// startWatch watches src in namespace ("" is all namespaces). mu is held.
func (s *InformerStore) startWatch(parent context.Context, src Source, namespace, selector, notice string) {
	r := s.newRun(parent, src, namespace, selector, notice, RefreshWatch)
	var example runtime.Object = &unstructured.Unstructured{}
	if src.Key == SourcePods {
		example = &corev1.Pod{}
	}
	inf := cache.NewSharedIndexInformer(&trackedListWatch{lw: s.listWatch(src, namespace, selector), run: r}, example, 0, cache.Indexers{})
	_ = inf.SetTransform(cache.TransformFunc(src.Transform))
	_ = inf.SetWatchErrorHandlerWithContext(func(_ context.Context, _ *cache.Reflector, err error) { r.onError(err) })
	_, _ = inf.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    func(any) { r.onEvent() },
		UpdateFunc: func(any, any) { r.onEvent() },
		DeleteFunc: func(any) { r.onEvent() },
	})
	r.informer = inf
	s.goFunc(func() { inf.RunWithContext(r.ctx) })
	s.goFunc(func() {
		if cache.WaitForCacheSync(r.ctx.Done(), inf.HasSynced) {
			r.onSynced()
		}
	})
}

// listWatch lists and watches src with the typed client for pods and the
// dynamic client otherwise.
func (s *InformerStore) listWatch(src Source, namespace, selector string) cache.ListerWatcherWithContext {
	withSelector := func(o metav1.ListOptions) metav1.ListOptions {
		o.LabelSelector = selector
		return o
	}
	if src.Key == SourcePods {
		pods := s.clients.Kube.CoreV1().Pods(namespace)
		return &cache.ListWatch{
			ListWithContextFunc: func(ctx context.Context, o metav1.ListOptions) (runtime.Object, error) {
				return pods.List(ctx, withSelector(o))
			},
			WatchFuncWithContext: func(ctx context.Context, o metav1.ListOptions) (watch.Interface, error) {
				return pods.Watch(ctx, withSelector(o))
			},
		}
	}
	res := s.clients.Dynamic.Resource(src.GVR).Namespace(namespace)
	return &cache.ListWatch{
		ListWithContextFunc: func(ctx context.Context, o metav1.ListOptions) (runtime.Object, error) {
			return res.List(ctx, withSelector(o))
		},
		WatchFuncWithContext: func(ctx context.Context, o metav1.ListOptions) (watch.Interface, error) {
			return res.Watch(ctx, withSelector(o))
		},
	}
}

// pollFunc reads a polled source once.
type pollFunc func(ctx context.Context) (SourceInfo, []*unstructured.Unstructured, []*corev1.Pod)

// startPoll polls src every interval. mu is held.
func (s *InformerStore) startPoll(parent context.Context, src Source, namespace, selector, notice string, interval time.Duration, read pollFunc) {
	r := s.newRun(parent, src, namespace, selector, notice, RefreshPoll)
	s.goFunc(func() {
		for {
			info, objs, pods := read(r.ctx)
			if r.ctx.Err() != nil {
				return
			}
			s.recordPoll(r, info, objs, pods, interval)
			t := time.NewTimer(interval)
			select {
			case <-r.ctx.Done():
				t.Stop()
				return
			case <-t.C:
			}
		}
	})
}

// listPoll is the pollFunc listing src in namespace.
func (s *InformerStore) listPoll(src Source, namespace, selector string) pollFunc {
	return func(ctx context.Context) (SourceInfo, []*unstructured.Unstructured, []*corev1.Pod) {
		if src.Key == SourcePods {
			info, pods := s.reader.readPods(ctx, src, namespace, selector)
			return info, nil, pods
		}
		info, objs := s.reader.readList(ctx, src, namespace, selector)
		return info, objs, nil
	}
}

// recordPoll stores the result of a poll. A failure after a success keeps
// the objects and is recorded as Disconnected.
func (s *InformerStore) recordPoll(r *sourceRun, info SourceInfo, objs []*unstructured.Unstructured, pods []*corev1.Pod, interval time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.current(r) {
		return
	}
	now := s.cfg.Now()
	prev := r.ls.info
	info.Mode = RefreshPoll
	info.NextPoll = now.Add(interval)
	info.Notice = cmp.Or(info.Notice, r.notice)
	switch {
	case info.State == SourceOK:
		info.LastOK = now
		r.objs, r.pods = sortObjects(objs), sortPods(pods)
	case info.State == SourceError && !prev.LastOK.IsZero():
		info.State, info.LastOK = SourceDisconnected, prev.LastOK
	case info.State == SourceError:
	default:
		r.objs, r.pods = nil, nil
	}
	r.ls.info = info
	s.notify()
	s.kickDependents(r.ls.src.Key)
}

// selectLoop selects the DDA whenever the DDAs change, and restarts the
// other sources when the selection moves to another DDA. While no single
// DDA is selected, the sources of the previous selection keep running;
// Build reports the missing or ambiguous DDA.
func (s *InformerStore) selectLoop() {
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-s.selectKick:
		}
		s.mu.Lock()
		if ls := s.sources[SourceDDA]; ls != nil && ls.run != nil && ls.info.State != SourceLoading {
			dda, err := selectDDA(ls.run.objectList(), s.cfg.DDAName)
			if err == nil {
				if sc := (scope{namespace: dda.GetNamespace(), dda: dda.GetName()}); sc != s.sel {
					s.startDependents(sc)
				}
			}
		}
		s.mu.Unlock()
	}
}

// startDependents (re)starts every source but the DDA for the selected
// DDA. mu is held.
func (s *InformerStore) startDependents(sc scope) {
	if s.depCancel != nil {
		s.depCancel()
	}
	for key, ls := range s.sources {
		if key != SourceDDA && ls.info.State != SourceNotInstalled {
			delete(s.sources, key)
		}
	}
	ctx, cancel := context.WithCancel(s.ctx)
	s.depCancel, s.sel = cancel, sc
	helmKick := make(chan struct{}, 1)
	s.helmKick.Store(&helmKick)

	for _, src := range Sources() {
		if src.Key == SourceDDA || src.Key == SourceLease || (src.Key == SourcePods && !s.cfg.Pods) {
			continue
		}
		if ls, ok := s.sources[src.Key]; ok && ls.info.State == SourceNotInstalled {
			continue
		}
		switch src.Mode {
		case RefreshWatch:
			namespace := sc.namespace
			if src.Scope == ScopeClusterFallback {
				namespace = metav1.NamespaceAll
			}
			s.startWatch(ctx, src, namespace, sc.selector(src), "")
		default:
			read := func(ctx context.Context) (SourceInfo, []*unstructured.Unstructured, []*corev1.Pod) {
				info, objs := s.reader.read(ctx, src, sc)
				if src.Key == SourceOperator {
					s.pollLease(ctx, info, objs, sc.namespace)
				}
				return info, objs, nil
			}
			s.startPoll(ctx, src, sc.namespace, sc.selector(src), "", s.cfg.PollInterval, read)
		}
	}
	s.goFunc(func() { s.helmLoop(ctx, helmKick) })
	kick(helmKick)
}

// pollLease reads the operator Lease in the namespace of the operator
// found by the operator poll. Without an operator there is no Lease source,
// as in the ListStore. When the operator poll fails, the Lease records the
// failure too: Disconnected with its objects kept once read.
func (s *InformerStore) pollLease(ctx context.Context, opInfo SourceInfo, operators []*unstructured.Unstructured, ddaNamespace string) {
	var src Source
	for _, candidate := range Sources() {
		if candidate.Key == SourceLease {
			src = candidate
		}
	}
	var op *unstructured.Unstructured
	if opInfo.State == SourceOK {
		op = pickOperator(operators, ddaNamespace)
	}
	s.mu.Lock()
	if ctx.Err() != nil {
		s.mu.Unlock()
		return
	}
	ls, exists := s.sources[SourceLease]
	switch {
	case exists && ls.info.State == SourceNotInstalled:
		s.mu.Unlock()
		return
	case opInfo.State == SourceError:
		var r *sourceRun
		if exists {
			r = ls.run
		}
		s.mu.Unlock()
		if r != nil {
			s.recordPoll(r, SourceInfo{State: SourceError, Err: opInfo.Err}, nil, nil, s.cfg.PollInterval)
		}
		return
	case op == nil:
		if exists {
			if ls.run != nil {
				ls.run.cancel()
			}
			delete(s.sources, SourceLease)
			s.notify()
		}
		s.mu.Unlock()
		return
	}
	r := ls.runIfAt(op.GetNamespace())
	if r == nil {
		r = s.newRun(ctx, src, op.GetNamespace(), "", "", RefreshPoll)
	}
	s.mu.Unlock()
	info, objs := s.reader.readNamed(ctx, src, op.GetNamespace())
	if ctx.Err() == nil {
		s.recordPoll(r, info, objs, nil, s.cfg.PollInterval)
	}
}

// runIfAt returns the run of ls when it reads namespace.
func (ls *liveSource) runIfAt(namespace string) *sourceRun {
	if ls == nil || ls.run == nil || ls.run.namespace != namespace {
		return nil
	}
	return ls.run
}

// helmLoop reads the Helm histories of the DDA and DAP releases when they
// change, and every HelmInterval while a rollout is in progress.
func (s *InformerStore) helmLoop(ctx context.Context, helmKick chan struct{}) {
	if s.clients.HelmHistory == nil {
		return
	}
	ticker := time.NewTicker(s.cfg.HelmInterval)
	defer ticker.Stop()
	last := ""
	for {
		periodic := false
		select {
		case <-ctx.Done():
			return
		case <-helmKick:
		case <-ticker.C:
			periodic = true
		}
		refs, fingerprint, ready := s.helmTargets()
		if !ready || (fingerprint == last && (!periodic || !s.rolloutInProgress())) {
			continue
		}
		if s.readHelmReleases(ctx, refs) {
			last = fingerprint
		}
	}
}

// helmTargets returns the Helm releases of the selected DDA and the DAPs,
// a fingerprint of their chart labels, and whether both sources are read.
func (s *InformerStore) helmTargets() ([]ReleaseRef, string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ddaSrc, dapSrc := s.sources[SourceDDA], s.sources[SourceDAP]
	if ddaSrc == nil || ddaSrc.run == nil || ddaSrc.info.State == SourceLoading || dapSrc == nil || dapSrc.info.State == SourceLoading {
		return nil, "", false
	}
	var dda *unstructured.Unstructured
	for _, d := range ddaSrc.run.objectList() {
		if d.GetNamespace() == s.sel.namespace && d.GetName() == s.sel.dda {
			dda = d
		}
	}
	if dda == nil {
		return nil, "", false
	}
	var daps []*unstructured.Unstructured
	if dapSrc.run != nil {
		daps = dapSrc.run.objectList()
	}
	var b strings.Builder
	for _, obj := range append([]*unstructured.Unstructured{dda}, daps...) {
		if rel, ok := HelmRelease(obj); ok {
			b.WriteString(rel.Key() + "=" + obj.GetLabels()[helmChartLabel] + ";")
		}
	}
	return helmReleases(dda, daps), b.String(), true
}

// readHelmReleases reads the releases in parallel. A failed read keeps the
// previous history of the release. It reports whether every release got an
// answer: a history, not found, or no access.
//
// The Helm storage drivers do not pass ctx to their API calls, so the reads
// run outside the store's goroutines: once ctx is done their results are
// dropped and Close does not wait for them.
func (s *InformerStore) readHelmReleases(ctx context.Context, refs []ReleaseRef) bool {
	var failed atomic.Bool
	history := func(ctx context.Context, namespace, release string) ([]helm.Revision, error) {
		revs, err := s.clients.HelmHistory(ctx, namespace, release)
		if err != nil && !errors.Is(err, helm.ErrReleaseNotFound) && !apierrors.IsForbidden(err) && !apierrors.IsUnauthorized(err) {
			failed.Store(true)
		}
		return revs, err
	}
	results := make([][]helm.Revision, len(refs))
	done := make(chan struct{})
	go func() {
		defer close(done)
		var wg sync.WaitGroup
		for i, rel := range refs {
			wg.Go(func() {
				if revs, ok := readHelm(ctx, history, rel); ok {
					results[i] = revs
				}
			})
		}
		wg.Wait()
	}()
	select {
	case <-ctx.Done():
		return false
	case <-done:
	}
	if ctx.Err() != nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	next := map[string][]helm.Revision{}
	for i, rel := range refs {
		switch {
		case results[i] != nil:
			next[rel.Key()] = results[i]
		case s.helm[rel.Key()] != nil:
			next[rel.Key()] = s.helm[rel.Key()]
		}
	}
	s.helm = next
	s.notify()
	return !failed.Load()
}

// rolloutInProgress reports whether an agent workload is rolling out: its
// spec is not observed yet or some of its pods are not updated. Unavailable
// pods alone are not a rollout.
func (s *InformerStore) rolloutInProgress() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, key := range []string{SourceDaemonSets, SourceDeployments} {
		ls := s.sources[key]
		if ls == nil || ls.run == nil {
			continue
		}
		for _, u := range ls.run.objectList() {
			if u.GetGeneration() > field(u, "status", "observedGeneration") {
				return true
			}
			var desired, updated int64
			if key == SourceDaemonSets {
				desired, updated = field(u, "status", "desiredNumberScheduled"), field(u, "status", "updatedNumberScheduled")
			} else {
				desired = 1
				if v, found := number(u, "spec", "replicas"); found {
					desired = v
				}
				updated = field(u, "status", "updatedReplicas")
			}
			if updated < desired {
				return true
			}
		}
	}
	return false
}

// field returns an integer field of u, or 0.
func field(u *unstructured.Unstructured, path ...string) int64 {
	v, _ := number(u, path...)
	return v
}

// number reads an integer field of u, whether decoded as an integer or as
// a float.
func number(u *unstructured.Unstructured, path ...string) (int64, bool) {
	v, found, _ := unstructured.NestedFieldNoCopy(u.Object, path...)
	switch n := v.(type) {
	case int64:
		return n, found
	case int32:
		return int64(n), found
	case int:
		return int64(n), found
	case float64:
		return int64(n), found
	default:
		return 0, false
	}
}

// objectList returns the objects of the run, sorted by namespace and name.
// mu is held or the run is not shared yet.
func (r *sourceRun) objectList() []*unstructured.Unstructured {
	if r.informer == nil {
		return r.objs
	}
	items := r.informer.GetStore().List()
	out := make([]*unstructured.Unstructured, 0, len(items))
	for _, item := range items {
		if u, ok := item.(*unstructured.Unstructured); ok {
			out = append(out, u)
		}
	}
	return sortObjects(out)
}

// podList returns the pods of the run, sorted by namespace and name.
func (r *sourceRun) podList() []*corev1.Pod {
	if r.informer == nil {
		return r.pods
	}
	items := r.informer.GetStore().List()
	out := make([]*corev1.Pod, 0, len(items))
	for _, item := range items {
		if p, ok := item.(*corev1.Pod); ok {
			out = append(out, p)
		}
	}
	return sortPods(out)
}

func sortObjects(objs []*unstructured.Unstructured) []*unstructured.Unstructured {
	slices.SortFunc(objs, func(a, b *unstructured.Unstructured) int {
		return cmp.Or(cmp.Compare(a.GetNamespace(), b.GetNamespace()), cmp.Compare(a.GetName(), b.GetName()))
	})
	return objs
}

func sortPods(pods []*corev1.Pod) []*corev1.Pod {
	slices.SortFunc(pods, func(a, b *corev1.Pod) int {
		return cmp.Or(cmp.Compare(a.Namespace, b.Namespace), cmp.Compare(a.Name, b.Name))
	})
	return pods
}

// onEvent handles an informer event. It must stay cheap: it only records
// the time and signals.
func (r *sourceRun) onEvent() {
	r.lastEvent.Store(r.s.cfg.Now().UnixNano())
	r.s.notify()
	r.s.kickDependents(r.ls.src.Key)
}

// onSynced marks the source OK once its informer has synced.
func (r *sourceRun) onSynced() {
	s := r.s
	s.mu.Lock()
	if s.current(r) && r.ls.info.State == SourceLoading {
		r.ls.info.State, r.ls.info.LastOK = SourceOK, s.cfg.Now()
		s.notify()
	}
	s.mu.Unlock()
	s.kickDependents(r.ls.src.Key)
}

// onRequest records the outcome of a list or watch request of the
// informer. A success after a failure recovers the source: OK once synced,
// Loading before.
func (r *sourceRun) onRequest(err error, isList bool) {
	if err != nil {
		r.onError(err)
		return
	}
	s := r.s
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.current(r) {
		return
	}
	info := &r.ls.info
	if info.State != SourceDisconnected && info.State != SourceError && info.State != SourceNotInstalled {
		return
	}
	switch {
	case r.informer.HasSynced():
		info.State, info.Err, info.LastOK = SourceOK, "", s.cfg.Now()
	case isList:
		info.State, info.Err = SourceLoading, ""
	default:
		return
	}
	s.notify()
}

// onError handles a failed list or watch. A forbidden request stops the
// informer and resolves the source in the background; other errors,
// including Unauthorized (an expired token can be renewed), mark it
// Disconnected once synced, or by their class before. The informer keeps
// retrying with backoff.
func (r *sourceRun) onError(err error) {
	if r.ctx.Err() != nil || apierrors.IsResourceExpired(err) || apierrors.IsGone(err) || errors.Is(err, io.EOF) {
		return
	}
	s := r.s
	state := classify(err)
	if apierrors.IsUnauthorized(err) {
		state = SourceError
	}
	if state == SourceForbidden {
		if r.resolving.CompareAndSwap(false, true) {
			r.cancel()
			s.goFunc(func() { s.resolveForbidden(r) })
		}
		return
	}
	if state == SourceError && r.informer.HasSynced() {
		state = SourceDisconnected
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.current(r) {
		return
	}
	if r.ls.info.State != state || r.ls.info.Err != err.Error() {
		r.ls.info.State, r.ls.info.Err = state, err.Error()
		s.notify()
	}
}

// resolveForbidden replaces a run whose request was forbidden. If the list
// is allowed, the watch is what is forbidden: the source is polled. A
// cluster-wide source whose list is forbidden is watched in the DDA
// namespace with a notice. If the list is forbidden otherwise, the source
// is Forbidden. A probe that fails for another reason is retried with
// backoff while the source shows the error.
func (s *InformerStore) resolveForbidden(r *sourceRun) {
	src := r.ls.src
	backoff := min(time.Second, s.cfg.FallbackInterval)
	var listErr error
	for {
		listErr = s.probe(r.parent, src, r.namespace, r.selector)
		if r.parent.Err() != nil {
			return
		}
		if listErr == nil || apierrors.IsForbidden(listErr) {
			break
		}
		if !s.recordProbeError(r, listErr) {
			return
		}
		t := time.NewTimer(backoff)
		select {
		case <-r.parent.Done():
			t.Stop()
			return
		case <-t.C:
		}
		backoff = min(2*backoff, s.cfg.FallbackInterval)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.current(r) {
		return
	}
	switch {
	case listErr == nil:
		s.startPoll(r.parent, src, r.namespace, r.selector, r.notice, s.cfg.FallbackInterval, s.listPoll(src, r.namespace, r.selector))
	case src.Scope == ScopeClusterFallback && r.namespace == metav1.NamespaceAll && s.sel.namespace != "":
		s.startWatch(r.parent, src, s.sel.namespace, r.selector, fallbackNotice(src, s.sel.namespace))
	default:
		r.ls.run = nil
		r.ls.info = SourceInfo{State: SourceForbidden, Mode: RefreshWatch, Notice: r.notice, Err: listErr.Error()}
		s.notify()
	}
}

// recordProbeError records a probe that failed for a reason other than
// Forbidden on the source of r, and reports whether r is still current.
func (s *InformerStore) recordProbeError(r *sourceRun, err error) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.current(r) {
		return false
	}
	state := classify(err)
	switch {
	case state == SourceNotInstalled:
	case r.informer.HasSynced() || !r.ls.info.LastOK.IsZero():
		state = SourceDisconnected
	default:
		state = SourceError
	}
	if r.ls.info.State != state || r.ls.info.Err != err.Error() {
		r.ls.info.State, r.ls.info.Err = state, err.Error()
		s.notify()
	}
	return true
}

// probe lists one object of src to tell a forbidden list from a forbidden
// watch.
func (s *InformerStore) probe(ctx context.Context, src Source, namespace, selector string) error {
	opts := metav1.ListOptions{LabelSelector: selector, Limit: 1}
	var err error
	if src.Key == SourcePods {
		_, err = s.clients.Kube.CoreV1().Pods(namespace).List(ctx, opts)
	} else {
		_, err = s.clients.Dynamic.Resource(src.GVR).Namespace(namespace).List(ctx, opts)
	}
	return err
}

// trackedListWatch reports the outcome of every list and watch request to
// its run, so that failures and recoveries reach the source state even when
// the reflector retries internally.
type trackedListWatch struct {
	lw  cache.ListerWatcherWithContext
	run *sourceRun
}

var (
	_ cache.ListerWatcher            = &trackedListWatch{}
	_ cache.ListerWatcherWithContext = &trackedListWatch{}
)

func (t *trackedListWatch) List(o metav1.ListOptions) (runtime.Object, error) {
	return t.ListWithContext(context.Background(), o)
}

func (t *trackedListWatch) Watch(o metav1.ListOptions) (watch.Interface, error) {
	return t.WatchWithContext(context.Background(), o)
}

func (t *trackedListWatch) ListWithContext(ctx context.Context, o metav1.ListOptions) (runtime.Object, error) {
	obj, err := t.lw.ListWithContext(ctx, o)
	t.run.onRequest(err, true)
	return obj, err
}

func (t *trackedListWatch) WatchWithContext(ctx context.Context, o metav1.ListOptions) (watch.Interface, error) {
	w, err := t.lw.WatchWithContext(ctx, o)
	t.run.onRequest(err, false)
	return w, err
}

// IsWatchListSemanticsUnSupported makes the reflector use a plain list,
// so that every list goes through ListWithContext.
func (t *trackedListWatch) IsWatchListSemanticsUnSupported() bool { return true }
