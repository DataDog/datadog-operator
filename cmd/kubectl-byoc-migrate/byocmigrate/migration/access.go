// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package migration

import (
	"context"
	"errors"
	"fmt"

	authorizationv1 "k8s.io/api/authorization/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
)

// checkPatchAccess asks the API server whether the user of the client may patch each object, as kubectl auth can-i
// does, and returns an error that lists every object that it may not patch. It checks the authorization only, not the
// admission of the patches.
func (m *Migration) checkPatchAccess(ctx context.Context, objects []client.Object) error {
	var errs []error
	for _, object := range objects {
		if err := m.checkPatch(ctx, object); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (m *Migration) checkPatch(ctx context.Context, object client.Object) error {
	gvk, err := apiutil.GVKForObject(object, m.Client.Scheme())
	if err != nil {
		return err
	}
	mapping, err := m.Client.RESTMapper().RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return fmt.Errorf("find the resource of %s: %w", m.name(object), err)
	}
	review := &authorizationv1.SelfSubjectAccessReview{Spec: authorizationv1.SelfSubjectAccessReviewSpec{
		ResourceAttributes: &authorizationv1.ResourceAttributes{
			Namespace: object.GetNamespace(),
			Verb:      "patch",
			Group:     mapping.Resource.Group,
			Version:   mapping.Resource.Version,
			Resource:  mapping.Resource.Resource,
			Name:      object.GetName(),
		},
	}}
	if err := m.Client.Create(ctx, review); err != nil {
		return fmt.Errorf("check the permission to patch %s: %w", m.name(object), err)
	}
	if review.Status.Allowed {
		return nil
	}
	denied := fmt.Errorf("%s: no permission to patch %s in namespace %s", m.name(object), mapping.Resource.GroupResource(), object.GetNamespace())
	if review.Status.Reason != "" {
		denied = fmt.Errorf("%w: %s", denied, review.Status.Reason)
	}
	return denied
}
