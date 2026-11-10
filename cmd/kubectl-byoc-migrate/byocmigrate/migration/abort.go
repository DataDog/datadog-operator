// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package migration

import (
	"context"
	"errors"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/cmd/kubectl-byoc-migrate/byocmigrate/converter"
)

// Abort reverts the migration, apart from the deletion of the cluster, which the user does, for example from a
// deployment pipeline. It checks first that the StatefulSet of each Observability Pipelines Worker manifest runs its
// Pods again, because deleting the cluster next deletes the Workers, and that the user may patch every object of the
// CloudPrem manifest that references the cluster, and writes nothing otherwise. It then removes the cluster from the
// owners of these objects. The controller cannot give the objects back to the cluster, because it applies an object
// only when the cluster controls it, and with the resource version that it read.
// Last, it checks that no object of the CloudPrem manifest still references the cluster, so that deleting the cluster
// deletes only the resources that the cluster created, such as the Workers. Applying the manifests again afterwards
// restores the settings of the charts.
//
// The objects are also removed from the owners of a cluster that is being deleted, so that the garbage collector does
// not delete them once the cluster is gone.
func (m *Migration) Abort(ctx context.Context, cloudprem converter.Manifest, pipelines []converter.PipelineManifest) error {
	cluster, err := m.getCluster(ctx)
	if err != nil {
		return err
	}
	switch {
	case cluster == nil:
		m.logf("DatadogBYOCCluster %s does not exist", m.Cluster)
	case !cluster.DeletionTimestamp.IsZero():
		m.logf("DatadogBYOCCluster %s is being deleted", m.Cluster)
		if err := m.releaseAll(ctx, cluster, cloudprem); err != nil {
			return err
		}
	default:
		if err := m.checkRestored(ctx, pipelines); err != nil {
			return err
		}
		if err := m.releaseAll(ctx, cluster, cloudprem); err != nil {
			return err
		}
		if err := m.checkReleased(ctx, cluster, cloudprem); err != nil {
			return err
		}
		m.logf("Delete DatadogBYOCCluster %s, for example from the deployment pipeline that applies it: it deletes only the resources that it created, such as the Workers", m.Cluster)
	}
	m.logf("Apply the manifests again, for example with helm upgrade and the same values, to restore the settings of the charts. With a server-side apply, as with Helm 4, force the conflicts, because the controller still manages the fields of the CloudPrem objects")
	return nil
}

// checkRestored checks that the StatefulSet of each Observability Pipelines Worker manifest has all its replicas
// ready, so that its clients can move back to it before the Workers are deleted with the cluster.
func (m *Migration) checkRestored(ctx context.Context, pipelines []converter.PipelineManifest) error {
	var errs []error
	for _, pipeline := range pipelines {
		statefulSet, err := converter.WorkerStatefulSet(pipeline.Manifest)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		live, err := m.getLive(ctx, statefulSet)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if live == nil {
			errs = append(errs, fmt.Errorf("%s does not exist; apply manifest %s again to recreate it", m.name(statefulSet), pipeline.Manifest.Name))
			continue
		}
		replicas := ptr.Deref(live.(*appsv1.StatefulSet).Spec.Replicas, 1)
		ready := live.(*appsv1.StatefulSet).Status.ReadyReplicas
		switch {
		case replicas == 0:
			errs = append(errs, fmt.Errorf("%s of manifest %s is scaled to zero; scale it back", m.name(statefulSet), pipeline.Manifest.Name))
		case ready < replicas:
			errs = append(errs, fmt.Errorf("%s of manifest %s has %d of %d replicas ready; wait for its Pods", m.name(statefulSet), pipeline.Manifest.Name, ready, replicas))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("deleting DatadogBYOCCluster %s deletes its Workers, so nothing was written; run the Pods of the manifests again and move their clients back to them, then run abort again:\n%w", m.Cluster, errors.Join(errs...))
	}
	return nil
}

// releaseAll checks that the user may patch every object of the CloudPrem manifest that references the cluster, then
// asks for the confirmation, and removes the cluster from the owners of these objects. It reads every object of the
// manifest, not only the objects that the cluster adopted, because the spec of the cluster may have changed since adopt.
func (m *Migration) releaseAll(ctx context.Context, cluster *datadoghqv1alpha1.DatadogBYOCCluster, cloudprem converter.Manifest) error {
	var owned []client.Object
	for _, object := range cloudprem.Objects {
		live, err := m.getLive(ctx, object)
		if err != nil {
			return err
		}
		if live != nil && referencesUID(live, cluster.UID) {
			owned = append(owned, live)
		}
	}
	if err := m.checkPatchAccess(ctx, owned); err != nil {
		return fmt.Errorf("the user of the Kubernetes context cannot remove DatadogBYOCCluster %s from the owners of the objects of manifest %s, so nothing was written; grant it the permissions, then run abort again:\n%w", m.Cluster, cloudprem.Name, err)
	}
	if err := m.confirm(); err != nil {
		return err
	}
	for _, object := range owned {
		if err := m.release(ctx, cluster, object); err != nil {
			return err
		}
	}
	return nil
}

// checkReleased checks that no object of the CloudPrem manifest still references the cluster, whose deletion would
// otherwise delete it.
func (m *Migration) checkReleased(ctx context.Context, cluster *datadoghqv1alpha1.DatadogBYOCCluster, cloudprem converter.Manifest) error {
	if m.DryRun {
		return nil
	}
	for _, object := range cloudprem.Objects {
		live, err := m.getLive(ctx, object)
		if err != nil {
			return err
		}
		if live != nil && referencesUID(live, cluster.UID) {
			return fmt.Errorf("%s still references DatadogBYOCCluster %s, so deleting the cluster deletes it; run abort again before deleting the cluster", m.name(object), m.Cluster)
		}
	}
	return nil
}

// release removes the cluster from the owners of the live object.
func (m *Migration) release(ctx context.Context, cluster *datadoghqv1alpha1.DatadogBYOCCluster, object client.Object) error {
	live, err := m.getLive(ctx, object)
	if err != nil || live == nil || !referencesUID(live, cluster.UID) {
		return err
	}
	m.logf("Removing DatadogBYOCCluster %s from the owners of %s", m.Cluster, m.name(object))
	if m.DryRun {
		return nil
	}
	patch, err := removeReferencesPatch(live, cluster.UID)
	if err != nil {
		return err
	}
	if err := m.Client.Patch(ctx, live, patch, client.FieldOwner(fieldManager)); err != nil {
		return fmt.Errorf("remove DatadogBYOCCluster %s from the owners of %s: %w", m.Cluster, m.name(object), err)
	}
	return nil
}
