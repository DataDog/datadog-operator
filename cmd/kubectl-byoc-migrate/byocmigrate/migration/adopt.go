// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package migration

import (
	"context"
	"errors"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/cmd/kubectl-byoc-migrate/byocmigrate/converter"
	byocutils "github.com/DataDog/datadog-operator/internal/controller/datadogbyoccluster/utils"
)

// LivePlan builds the plan of the live DatadogBYOCCluster, which the user applies before adopt, for example from a
// deployment pipeline, so that adopt checks the resources that the controller applies.
func (m *Migration) LivePlan(ctx context.Context, cloudprem converter.Manifest, pipelines []converter.PipelineManifest) (*converter.Plan, error) {
	cluster, err := m.getCluster(ctx)
	switch {
	case err != nil:
		return nil, err
	case cluster == nil:
		return nil, fmt.Errorf("DatadogBYOCCluster %s does not exist; apply the cluster that convert printed, for example from a deployment pipeline, then run the command again", m.Cluster)
	}
	return m.livePlan(cluster, cloudprem, pipelines)
}

func (m *Migration) livePlan(cluster *datadoghqv1alpha1.DatadogBYOCCluster, cloudprem converter.Manifest, pipelines []converter.PipelineManifest) (*converter.Plan, error) {
	switch {
	case !cluster.DeletionTimestamp.IsZero():
		return nil, fmt.Errorf("DatadogBYOCCluster %s is being deleted", m.Cluster)
	case cluster.Spec.Datadog == nil || cluster.Spec.Components == nil:
		return nil, fmt.Errorf("DatadogBYOCCluster %s has no spec.datadog or spec.components", m.Cluster)
	}
	return converter.NewPlan(cluster, cloudprem, pipelines)
}

// Adopt gives the CloudPrem resources of the manifest to the live DatadogBYOCCluster, whose plan LivePlan builds and
// must have no blocker. It checks that every object of the manifest that the cluster adopts exists and has no other
// controller, and that the user may patch it, and writes nothing when a check fails. It does not compare the objects with the resources of the
// cluster: Diff does. It then gives each resource to the cluster. The controller applies a resource once the cluster
// controls it, and reports a conflict for the others until then. It creates the Workers only once it applied every
// other resource.
func (m *Migration) Adopt(ctx context.Context, plan *converter.Plan) error {
	cluster := plan.Cluster
	targets, err := m.adoptionTargets(ctx, plan, cluster)
	if err != nil {
		return fmt.Errorf("the DatadogBYOCCluster cannot adopt the resources of manifest %s, so nothing was written:\n%w", plan.CloudPrem.Name, err)
	}
	if err = m.checkPatchAccess(ctx, targets); err != nil {
		return fmt.Errorf("the user of the Kubernetes context cannot give the resources of manifest %s to the DatadogBYOCCluster, so nothing was written; grant it the permissions, then run adopt again:\n%w", plan.CloudPrem.Name, err)
	}
	m.logf("DatadogBYOCCluster %s adopts %d resources of manifest %s", m.Cluster, len(targets), plan.CloudPrem.Name)
	for _, target := range targets {
		m.logf("  %s", m.name(target))
	}
	if len(targets) == 0 {
		return nil
	}
	if err = m.confirm(); err != nil {
		return err
	}

	for _, target := range targets {
		if err := m.adoptObject(ctx, cluster, target); err != nil {
			return fmt.Errorf("adopt %s: %w; run adopt again", m.name(target), err)
		}
	}
	return nil
}

// adoptionTargets returns the desired resources whose live objects the cluster adopts. It fails without writing when
// an object of the manifest that the cluster adopts does not exist, which happens when the command runs against
// another cluster or namespace than the manifest, or when a desired resource exists and is neither controlled by the
// cluster nor adoptable from the manifest.
func (m *Migration) adoptionTargets(ctx context.Context, plan *converter.Plan, cluster *datadoghqv1alpha1.DatadogBYOCCluster) ([]client.Object, error) {
	var targets []client.Object
	var errs []error
	for _, desired := range plan.Desired {
		live, err := m.getLive(ctx, desired)
		switch {
		case err != nil:
			errs = append(errs, err)
		case live == nil && inManifest(plan, desired):
			errs = append(errs, m.missingError(plan, desired))
		case live == nil, metav1.IsControlledBy(live, cluster):
		case !adoptable(live):
			errs = append(errs, fmt.Errorf("%s %w", m.name(desired), errNotAdoptable))
		case !inManifest(plan, desired):
			errs = append(errs, fmt.Errorf("%s exists, but it is not an object of manifest %s", m.name(desired), plan.CloudPrem.Name))
		default:
			targets = append(targets, desired)
		}
	}
	return targets, errors.Join(errs...)
}

// missingError reports an object of the manifest that the cluster adopts, but that does not exist.
func (m *Migration) missingError(plan *converter.Plan, desired client.Object) error {
	return fmt.Errorf("%s of manifest %s does not exist in namespace %s; check the context and the namespace of the command, and that the manifest is the one that was applied", m.name(desired), plan.CloudPrem.Name, desired.GetNamespace())
}

func adoptable(object client.Object) bool {
	return metav1.GetControllerOfNoCopy(object) == nil && object.GetDeletionTimestamp().IsZero()
}

// inManifest reports whether desired is a resource that the plan adopts from the CloudPrem manifest.
func inManifest(plan *converter.Plan, desired client.Object) bool {
	for _, planned := range plan.ObjectsWith(converter.ActionAdopt) {
		if planned.Desired == desired {
			return true
		}
	}
	return false
}

// adoptObject gives the fields of the live object to the field manager of the controller, then makes the cluster its
// controller. Services keep the fields of the managers other than the tools that apply the manifests, because the
// cluster cannot express load balancer settings (givesFields). The cluster adopts the object only once both writes are
// done, so a retry after a conflict starts over.
func (m *Migration) adoptObject(ctx context.Context, cluster *datadoghqv1alpha1.DatadogBYOCCluster, desired client.Object) error {
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error { return m.giveToCluster(ctx, cluster, desired) })
}

func (m *Migration) giveToCluster(ctx context.Context, cluster *datadoghqv1alpha1.DatadogBYOCCluster, desired client.Object) error {
	live, err := m.getLive(ctx, desired)
	switch {
	case err != nil:
		return err
	case live == nil, metav1.IsControlledBy(live, cluster):
		// The controller creates a missing object.
		return nil
	case !adoptable(live):
		return fmt.Errorf("%s %w", m.name(desired), errNotAdoptable)
	}
	if err = m.giveFields(ctx, live); err != nil {
		return err
	}
	patch, err := controllerReferencePatch(live, cluster)
	if err != nil {
		return err
	}
	return m.Client.Patch(ctx, live, patch, client.FieldOwner(fieldManager))
}

// giveFields gives the fields of the live object to the field manager of the controller, and updates live.
func (m *Migration) giveFields(ctx context.Context, live client.Object) error {
	gvk, err := apiutil.GVKForObject(live, m.Client.Scheme())
	if err != nil {
		return err
	}
	patch, err := managedFieldsPatch(live, byocutils.ClusterFieldOwner, gvk.GroupVersion().String())
	if err != nil {
		return err
	}
	return m.Client.Patch(ctx, live, patch)
}
