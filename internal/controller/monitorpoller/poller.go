// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package monitorpoller polls the state of Datadog monitors on behalf of the
// custom resources that report it. A single worker fetches the registered
// monitors in batches with the monitor list API, instead of each custom
// resource fetching its own monitor, and then enqueues the custom resources
// whose monitors it fetched so that their controllers update their status.
package monitorpoller

import (
	"context"
	"maps"
	"slices"
	"sync"
	"time"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/controller/priorityqueue"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	// DefaultInterval is how often registered monitors are polled when
	// Options.Interval is unset.
	DefaultInterval = 60 * time.Second
	// MaxBatchSize is the largest number of monitors fetched per request, and
	// the default. It is the largest page size the monitor list API accepts.
	MaxBatchSize = 1000

	// defaultNewMonitorDelay is how long the first poll of newly registered
	// monitors waits for more registrations to share its request, for example
	// when every custom resource registers after a restart.
	defaultNewMonitorDelay = 5 * time.Second
	// registrationTTLIntervals is how many polling intervals a registration
	// stays active without being refreshed. Each poll notifies subscribers,
	// whose reconciles refresh their registrations, so expiry only reclaims
	// registrations of custom resources that disappeared without unregistering.
	registrationTTLIntervals = 10
)

// Options configures a Poller.
type Options struct {
	// Interval is how often every registered monitor is polled.
	Interval time.Duration
	// BatchSize is the maximum number of monitors fetched per request.
	BatchSize int
	// NewMonitorDelay is how long the first poll of newly registered monitors
	// waits for more registrations to share its request.
	NewMonitorDelay time.Duration
}

// Subscriber identifies a custom resource that reports the state of a monitor.
type Subscriber struct {
	// Consumer names the controller that reconciles the custom resource.
	// Notifications are delivered through the Source of the same name.
	Consumer string
	types.NamespacedName
}

// Result is the outcome of polling a monitor.
type Result struct {
	// Monitor is the monitor returned by Datadog, with the state of its
	// triggered groups. It is empty when Err is set.
	Monitor datadogV1.Monitor
	// Err is set when the monitor could not be fetched. It contains
	// utils.NotFoundString only when Datadog reported that the monitor does
	// not exist.
	Err error
	// PolledAt is when the poll that produced the result started.
	PolledAt time.Time
}

// Poller periodically fetches the monitors that subscribers registered and
// notifies the subscribers of the results. Monitors are polled per scope, that
// is per set of credentials and Datadog site, so that each request only covers
// monitors of a single Datadog organization.
type Poller struct {
	client          *datadogV1.MonitorsApi
	log             logr.Logger
	interval        time.Duration
	batchSize       int
	newMonitorDelay time.Duration
	now             func() time.Time
	// wake interrupts the wait for the next poll when new monitors register.
	wake chan struct{}

	mu            sync.Mutex
	scopes        map[scopeKey]*scope
	registrations map[Subscriber]*registration
	notifiers     map[string]func(types.NamespacedName)
}

type scope struct {
	creds       credentials
	subscribers map[int64]map[Subscriber]struct{}
	results     map[int64]Result
	// nextPoll is when every monitor of the scope is polled next.
	nextPoll time.Time
	// newMonitors are registered monitors without a result yet. They are
	// polled at newMonitorsPollAt rather than waiting for nextPoll.
	newMonitors       map[int64]struct{}
	newMonitorsPollAt time.Time
}

type registration struct {
	scope     scopeKey
	monitorID int64
	lastSeen  time.Time
}

type pollJob struct {
	scope scopeKey
	creds credentials
	ids   []int64
}

// New returns a Poller that fetches monitors with client.
func New(client *datadogV1.MonitorsApi, log logr.Logger, opts Options) *Poller {
	if opts.Interval <= 0 {
		opts.Interval = DefaultInterval
	}
	if opts.BatchSize <= 0 || opts.BatchSize > MaxBatchSize {
		opts.BatchSize = MaxBatchSize
	}
	if opts.NewMonitorDelay <= 0 {
		opts.NewMonitorDelay = defaultNewMonitorDelay
	}

	return &Poller{
		client:          client,
		log:             log,
		interval:        opts.Interval,
		batchSize:       opts.BatchSize,
		newMonitorDelay: opts.NewMonitorDelay,
		now:             time.Now,
		wake:            make(chan struct{}, 1),
		scopes:          map[scopeKey]*scope{},
		registrations:   map[Subscriber]*registration{},
		notifiers:       map[string]func(types.NamespacedName){},
	}
}

// Start polls registered monitors until ctx is cancelled. The Poller is a
// manager.Runnable that requires leader election, like the controllers it
// serves.
func (p *Poller) Start(ctx context.Context) error {
	p.log.Info("Starting monitor state poller", "interval", p.interval.String(), "batchSize", p.batchSize)

	timer := time.NewTimer(p.poll(ctx))
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-p.wake:
		case <-timer.C:
		}
		timer.Reset(p.poll(ctx))
	}
}

// Source returns a source that enqueues the custom resources of consumer whose
// monitor was polled, so that their controller applies the new result.
func (p *Poller) Source(consumer string) source.Source {
	return source.Func(func(_ context.Context, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) error {
		p.mu.Lock()
		defer p.mu.Unlock()
		p.notifiers[consumer] = func(key types.NamespacedName) {
			request := reconcile.Request{NamespacedName: key}
			// State refreshes are background work, which must not delay
			// lifecycle and specification changes in the work queue.
			if priorityQueue, ok := queue.(priorityqueue.PriorityQueue[reconcile.Request]); ok {
				priorityQueue.AddWithOpts(priorityqueue.AddOpts{Priority: ptr.To(handler.LowPriority)}, request)
				return
			}
			queue.Add(request)
		}
		return nil
	})
}

// Register makes sub report the state of monitorID, polled with the
// credentials of the Datadog API context auth, in place of any monitor sub
// reported before. Registrations expire unless they are refreshed by
// registering again, which subscribers do when reconciling after each
// notification.
func (p *Poller) Register(auth context.Context, sub Subscriber, monitorID int64) {
	creds := credentialsFrom(auth)
	key := creds.scope()
	now := p.now()

	p.mu.Lock()
	defer p.mu.Unlock()

	if reg, ok := p.registrations[sub]; ok {
		if reg.scope == key && reg.monitorID == monitorID {
			reg.lastSeen = now
			return
		}
		p.unregisterLocked(sub, reg)
	}

	sc, ok := p.scopes[key]
	if !ok {
		sc = &scope{
			creds:       creds,
			subscribers: map[int64]map[Subscriber]struct{}{},
			results:     map[int64]Result{},
			nextPoll:    now.Add(p.interval),
			newMonitors: map[int64]struct{}{},
		}
		p.scopes[key] = sc
	}

	subscribers, ok := sc.subscribers[monitorID]
	if !ok {
		subscribers = map[Subscriber]struct{}{}
		sc.subscribers[monitorID] = subscribers
		if len(sc.newMonitors) == 0 {
			sc.newMonitorsPollAt = now.Add(p.newMonitorDelay)
		}
		sc.newMonitors[monitorID] = struct{}{}
		select {
		case p.wake <- struct{}{}:
		default:
		}
	}
	subscribers[sub] = struct{}{}
	p.registrations[sub] = &registration{scope: key, monitorID: monitorID, lastSeen: now}
}

// Unregister stops polling on behalf of sub.
func (p *Poller) Unregister(sub Subscriber) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if reg, ok := p.registrations[sub]; ok {
		p.unregisterLocked(sub, reg)
	}
}

// Result returns the latest result of polling monitorID with the credentials
// of the Datadog API context auth, if the monitor has been polled since it
// was registered.
func (p *Poller) Result(auth context.Context, monitorID int64) (Result, bool) {
	key := credentialsFrom(auth).scope()

	p.mu.Lock()
	defer p.mu.Unlock()

	sc, ok := p.scopes[key]
	if !ok {
		return Result{}, false
	}
	result, ok := sc.results[monitorID]
	return result, ok
}

func (p *Poller) unregisterLocked(sub Subscriber, reg *registration) {
	delete(p.registrations, sub)

	sc, ok := p.scopes[reg.scope]
	if !ok {
		return
	}
	subscribers := sc.subscribers[reg.monitorID]
	delete(subscribers, sub)
	if len(subscribers) > 0 {
		return
	}
	delete(sc.subscribers, reg.monitorID)
	delete(sc.results, reg.monitorID)
	delete(sc.newMonitors, reg.monitorID)
	if len(sc.subscribers) == 0 {
		delete(p.scopes, reg.scope)
	}
}

// poll fetches the monitors that are due, notifies their subscribers and
// returns how long to wait until more monitors are due.
func (p *Poller) poll(ctx context.Context) time.Duration {
	for _, job := range p.dueJobs() {
		if ctx.Err() != nil {
			break
		}
		results := p.fetch(ctx, job.creds, job.ids)
		p.notify(p.storeResults(job.scope, results))
	}
	return p.untilNextPoll()
}

func (p *Poller) dueJobs() []pollJob {
	now := p.now()

	p.mu.Lock()
	defer p.mu.Unlock()

	ttl := registrationTTLIntervals * p.interval
	for sub, reg := range p.registrations {
		if now.Sub(reg.lastSeen) > ttl {
			p.log.V(1).Info("Monitor state registration expired", "consumer", sub.Consumer, "namespace", sub.Namespace, "name", sub.Name, "monitorID", reg.monitorID)
			p.unregisterLocked(sub, reg)
		}
	}

	var jobs []pollJob
	for key, sc := range p.scopes {
		switch {
		case !now.Before(sc.nextPoll):
			jobs = append(jobs, pollJob{scope: key, creds: sc.creds, ids: slices.Sorted(maps.Keys(sc.subscribers))})
			sc.nextPoll = now.Add(p.interval)
			clear(sc.newMonitors)
		case len(sc.newMonitors) > 0 && !now.Before(sc.newMonitorsPollAt):
			jobs = append(jobs, pollJob{scope: key, creds: sc.creds, ids: slices.Sorted(maps.Keys(sc.newMonitors))})
			clear(sc.newMonitors)
		}
	}
	return jobs
}

// storeResults records results for the monitors that are still registered and
// returns their subscribers.
func (p *Poller) storeResults(key scopeKey, results map[int64]Result) []Subscriber {
	p.mu.Lock()
	defer p.mu.Unlock()

	sc, ok := p.scopes[key]
	if !ok {
		return nil
	}
	var subscribers []Subscriber
	for monitorID, result := range results {
		monitorSubscribers, ok := sc.subscribers[monitorID]
		if !ok {
			continue
		}
		sc.results[monitorID] = result
		for sub := range monitorSubscribers {
			subscribers = append(subscribers, sub)
		}
	}
	return subscribers
}

func (p *Poller) notify(subscribers []Subscriber) {
	p.mu.Lock()
	notifiers := maps.Clone(p.notifiers)
	p.mu.Unlock()

	for _, sub := range subscribers {
		if notify, ok := notifiers[sub.Consumer]; ok {
			notify(sub.NamespacedName)
		}
	}
}

func (p *Poller) untilNextPoll() time.Duration {
	now := p.now()

	p.mu.Lock()
	defer p.mu.Unlock()

	wait := p.interval
	for _, sc := range p.scopes {
		wait = min(wait, sc.nextPoll.Sub(now))
		if len(sc.newMonitors) > 0 {
			wait = min(wait, sc.newMonitorsPollAt.Sub(now))
		}
	}
	return max(wait, 0)
}
