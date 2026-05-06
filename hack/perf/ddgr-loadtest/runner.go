// hack/perf/ddgr-loadtest/runner.go
package main

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
)

const (
	labelKey   = "loadtest"
	labelValue = "ddgr-perf"
)

type Runner struct {
	cfg Config
	cli client.Client
}

func NewRunner(cfg Config) (*Runner, error) {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(v1alpha1.AddToScheme(scheme))

	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	if cfg.Kubeconfig != "" {
		loadingRules.ExplicitPath = cfg.Kubeconfig
	}
	rest, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		loadingRules, &clientcmd.ConfigOverrides{},
	).ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("kubeconfig: %w", err)
	}
	cli, err := client.New(rest, client.Options{Scheme: scheme})
	if err != nil {
		return nil, fmt.Errorf("k8s client: %w", err)
	}
	return &Runner{cfg: cfg, cli: cli}, nil
}

func (r *Runner) ensureNamespace(ctx context.Context) error {
	var ns corev1.Namespace
	err := r.cli.Get(ctx, client.ObjectKey{Name: r.cfg.Namespace}, &ns)
	if err == nil {
		return nil
	}
	if !apierrors.IsNotFound(err) {
		return err
	}
	create := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: r.cfg.Namespace}}
	return r.cli.Create(ctx, create)
}

func (r *Runner) ddgrName(i int) string {
	return fmt.Sprintf("%s-%04d", r.cfg.NamePrefix, i)
}

// Cleanup deletes all DDGRs in the test namespace with the loadtest label
// and waits up to 10 minutes for them to drain (the operator's finalizer
// must DELETE each from the Datadog API before the resource leaves etcd).
func (r *Runner) Cleanup(ctx context.Context) error {
	log.Printf("phase=cleanup namespace=%s label=%s=%s", r.cfg.Namespace, labelKey, labelValue)
	err := r.cli.DeleteAllOf(ctx, &v1alpha1.DatadogGenericResource{},
		client.InNamespace(r.cfg.Namespace),
		client.MatchingLabels{labelKey: labelValue},
	)
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("delete-collection: %w", err)
	}

	deadline := time.Now().Add(10 * time.Minute)
	for time.Now().Before(deadline) {
		var list v1alpha1.DatadogGenericResourceList
		if err := r.cli.List(ctx, &list,
			client.InNamespace(r.cfg.Namespace),
			client.MatchingLabels{labelKey: labelValue},
		); err != nil {
			return fmt.Errorf("list during drain: %w", err)
		}
		if len(list.Items) == 0 {
			log.Printf("phase=cleanup-complete remaining=0")
			return nil
		}
		log.Printf("phase=cleanup remaining=%d", len(list.Items))
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(10 * time.Second):
		}
	}
	return fmt.Errorf("cleanup timed out; some DDGRs may remain")
}

// Fill creates Count DDGRs in parallel with bounded concurrency.
// Each DDGR is labeled loadtest=ddgr-perf and named <prefix>-<i:04d>.
// Already-existing DDGRs are tolerated (idempotent restart).
func (r *Runner) Fill(ctx context.Context) error {
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(r.cfg.FillConcurrency)
	var mu sync.Mutex
	done := 0
	for i := 0; i < r.cfg.Count; i++ {
		i := i
		g.Go(func() error {
			ddgr := &v1alpha1.DatadogGenericResource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      r.ddgrName(i),
					Namespace: r.cfg.Namespace,
					Labels:    map[string]string{labelKey: labelValue},
				},
				Spec: v1alpha1.DatadogGenericResourceSpec{
					Type:     v1alpha1.Monitor,
					JsonSpec: BuildMonitorJSON(i, 0),
				},
			}
			if err := r.cli.Create(gctx, ddgr); err != nil && !apierrors.IsAlreadyExists(err) {
				return fmt.Errorf("create %s: %w", ddgr.Name, err)
			}
			mu.Lock()
			done++
			d := done
			mu.Unlock()
			if d%50 == 0 || d == r.cfg.Count {
				log.Printf("phase=fill created=%d/%d", d, r.cfg.Count)
			}
			return nil
		})
	}
	return g.Wait()
}
