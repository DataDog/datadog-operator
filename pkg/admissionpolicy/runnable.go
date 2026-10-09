// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package admissionpolicy

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	admregv1 "k8s.io/api/admissionregistration/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/discovery"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/DataDog/datadog-operator/pkg/celvalidation"
)

const (
	// initialBackoff is the first retry delay while the initial apply has not
	// yet succeeded.
	initialBackoff = 2 * time.Second
	// maxBackoff caps the retry delay for the initial apply.
	maxBackoff = 1 * time.Minute
	// refreshInterval is how often the policy is re-applied after the first
	// success. It re-converges the objects if they are edited or deleted
	// out-of-band, and picks up a cluster that gains the API after the operator
	// started, which the startup PlatformInfo snapshot never would.
	refreshInterval = 10 * time.Minute
)

// Controller keeps the ValidatingAdmissionPolicy objects and their bindings
// converged, on clusters that serve them.
//
// It is a leader-elected manager Runnable rather than part of the DatadogAgent
// dependency store: the store is per-DatadogAgent and ownership-based, and these
// objects are cluster-wide singletons with no owning DatadogAgent. Leader
// election keeps a multi-replica operator from having several writers.
type Controller struct {
	// apiClient is uncached on purpose. The manager cache would otherwise start
	// an informer for a type the cluster may not serve at all.
	apiClient client.Client
	discovery discovery.DiscoveryInterface
	logger    logr.Logger

	// supported records the last capability check, so an unsupported cluster
	// logs once rather than on every refresh.
	supported *bool
}

// Compile-time checks that Controller is a leader-only manager Runnable.
var (
	_ manager.Runnable               = &Controller{}
	_ manager.LeaderElectionRunnable = &Controller{}
)

// NewController builds the policy Controller from the manager. It creates its own
// uncached client; see the apiClient field.
func NewController(mgr manager.Manager, logger logr.Logger) (*Controller, error) {
	apiClient, err := client.New(mgr.GetConfig(), client.Options{Scheme: mgr.GetScheme()})
	if err != nil {
		return nil, fmt.Errorf("unable to build admission policy client: %w", err)
	}
	dc, err := discovery.NewDiscoveryClientForConfig(mgr.GetConfig())
	if err != nil {
		return nil, fmt.Errorf("unable to build admission policy discovery client: %w", err)
	}
	return &Controller{
		apiClient: apiClient,
		discovery: dc,
		logger:    logger.WithName("admission-policy"),
	}, nil
}

// NeedLeaderElection implements manager.LeaderElectionRunnable.
func (c *Controller) NeedLeaderElection() bool { return true }

// Start implements manager.Runnable. It retries the initial apply with bounded
// backoff, then re-applies periodically. It returns when ctx is cancelled
// (leadership lost or shutdown).
func (c *Controller) Start(ctx context.Context) error {
	c.logger = ctrl.LoggerFrom(ctx).WithName("admission-policy")
	c.logger.Info("Starting admission policy controller", "refresh", refreshInterval)

	backoff := initialBackoff
	for {
		if err := c.apply(ctx); err == nil {
			break
		} else {
			c.logger.Error(err, "Unable to apply admission policy, will retry", "backoff", backoff)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(backoff):
			if backoff *= 2; backoff > maxBackoff {
				backoff = maxBackoff
			}
		}
	}

	ticker := time.NewTicker(refreshInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			c.logger.Info("Stopping admission policy controller")
			return nil
		case <-ticker.C:
			if err := c.apply(ctx); err != nil {
				c.logger.Error(err, "Unable to apply admission policy")
			}
		}
	}
}

// apply converges the policy and its binding, after confirming the cluster
// serves them. An unsupported cluster is not an error.
func (c *Controller) apply(ctx context.Context) error {
	ok, err := c.policiesSupported()
	if err != nil {
		return err
	}
	if !ok {
		if c.supported == nil || *c.supported {
			c.logger.Info("Cluster does not serve admissionregistration.k8s.io/v1 validatingadmissionpolicies, skipping admission policy",
				"requiredKubernetesVersion", celvalidation.CompatibilityVersion.String())
		}
		c.supported = &ok
		return nil
	}
	if c.supported == nil || !*c.supported {
		c.logger.Info("Applying admission policies", "action", "Warn")
	}
	c.supported = &ok

	// Each policy is applied before its binding: a binding naming a policy that
	// does not exist is inert, whereas a policy with no binding is simply not
	// enforced. Ordering it this way means no window where the binding
	// references something missing.
	for _, t := range Targets() {
		if err := c.serverSideApply(ctx, BuildPolicy(t)); err != nil {
			return fmt.Errorf("unable to apply ValidatingAdmissionPolicy %s: %w", t.name, err)
		}
		if err := c.serverSideApply(ctx, BuildBinding(t)); err != nil {
			return fmt.Errorf("unable to apply ValidatingAdmissionPolicyBinding %s: %w", t.name, err)
		}
	}
	return nil
}

// serverSideApply applies obj with the operator as field owner. Server-side
// apply converges without a read-modify-write race and leaves fields the
// operator does not set alone.
func (c *Controller) serverSideApply(ctx context.Context, obj client.Object) error {
	return c.apiClient.Patch(ctx, obj, client.Apply, client.FieldOwner(FieldOwner), client.ForceOwnership)
}

// policiesSupported reports whether the cluster serves
// admissionregistration.k8s.io/v1 validatingadmissionpolicies. It queries
// discovery on each call rather than using kubernetes.PlatformInfo, which is a
// one-shot snapshot taken at operator startup and would never observe a cluster
// that gained the API later.
func (c *Controller) policiesSupported() (bool, error) {
	gv := admregv1.SchemeGroupVersion.String()
	resources, err := c.discovery.ServerResourcesForGroupVersion(gv)
	if err != nil {
		if apierrors.IsNotFound(err) || discovery.IsGroupDiscoveryFailedError(err) {
			return false, nil
		}
		return false, fmt.Errorf("unable to discover %s: %w", gv, err)
	}
	var policies, bindings bool
	for _, r := range resources.APIResources {
		switch r.Name {
		case "validatingadmissionpolicies":
			policies = true
		case "validatingadmissionpolicybindings":
			bindings = true
		}
	}
	return policies && bindings, nil
}

// Cleanup deletes the policy and its binding. Nothing calls it today: the
// objects are cluster-scoped and un-owned, so there is no owner whose deletion
// would collect them, and the DatadogAgent finalizer is the wrong home for an
// operator-wide singleton. Whether uninstall cleanup belongs here, in Helm, or
// in OLM is an open question; the function exists so the decision is not blocked
// on writing it.
func (c *Controller) Cleanup(ctx context.Context) error {
	for _, t := range Targets() {
		for _, obj := range []client.Object{BuildBinding(t), BuildPolicy(t)} {
			if err := c.apiClient.Delete(ctx, obj); err != nil && !apierrors.IsNotFound(err) {
				return err
			}
		}
	}
	return nil
}
