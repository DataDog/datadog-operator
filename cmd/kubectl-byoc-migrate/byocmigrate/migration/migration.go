// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package migration migrates the objects of the CloudPrem and Observability Pipelines Worker Helm charts to a
// DatadogBYOCCluster:
//   - Adopt gives the CloudPrem objects to the cluster,
//   - and Abort takes them back from the cluster.
//
// The commands do not create, change, or delete the cluster, which the user applies and deletes, for example from a
// deployment pipeline.
//
// The DatadogBYOCCluster controller does not take part in the migration, and keeps reconciling the cluster while the
// commands change the ownership of its resources. It applies an object only when the cluster controls the object it
// read, and with the resource version of that read, so it does not write to an object that the cluster does not
// control yet, nor give back an object that abort removed from the owners of the cluster.
package migration

import (
	"context"
	"errors"
	"fmt"
	"io"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
)

// fieldManager is the field manager of the writes of the commands, apart from the fields given to the cluster.
const fieldManager = "kubectl-byoc-migrate"

// Migration migrates the manifests to a DatadogBYOCCluster. Each command reads the manifests and the live objects again,
// so a command that stopped halfway can run again.
type Migration struct {
	Client client.Client
	// Cluster is the key of the DatadogBYOCCluster.
	Cluster types.NamespacedName
	// Out receives the progress of the commands.
	Out io.Writer
	// DryRun runs the checks of Abort and prints what it would change, without writing anything.
	DryRun bool
	// Confirm, if set, is called once the checks of a command pass, before its first write, and stops the command
	// when it returns an error.
	Confirm func() error
}

// confirm asks for a confirmation before the first write of a command.
func (m *Migration) confirm() error {
	if m.DryRun || m.Confirm == nil {
		return nil
	}
	return m.Confirm()
}

func (m *Migration) logf(format string, args ...any) {
	fmt.Fprintf(m.Out, format+"\n", args...)
}

// getCluster returns the live DatadogBYOCCluster, or nil when it does not exist.
func (m *Migration) getCluster(ctx context.Context) (*datadoghqv1alpha1.DatadogBYOCCluster, error) {
	cluster := &datadoghqv1alpha1.DatadogBYOCCluster{}
	if err := m.Client.Get(ctx, m.Cluster, cluster); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("get DatadogBYOCCluster %s: %w", m.Cluster, err)
	}
	return cluster, nil
}

// name returns the kind and name of an object for messages.
func (m *Migration) name(object client.Object) string {
	gvk, err := apiutil.GVKForObject(object, m.Client.Scheme())
	if err != nil {
		return fmt.Sprintf("%T/%s", object, object.GetName())
	}
	return gvk.Kind + "/" + object.GetName()
}

// getLive reads the live object with the name of object, or returns nil when it does not exist, including when the API
// server no longer serves its kind, such as a ServiceMonitor after its CRD was deleted.
func (m *Migration) getLive(ctx context.Context, object client.Object) (client.Object, error) {
	live := object.DeepCopyObject().(client.Object)
	if err := m.Client.Get(ctx, client.ObjectKeyFromObject(object), live); err != nil {
		if apierrors.IsNotFound(err) || meta.IsNoMatchError(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("get %s: %w", m.name(object), err)
	}
	return live, nil
}

var errNotAdoptable = errors.New("is controlled by another owner or is being deleted")
