package k8s

import (
	"context"
	"fmt"
	"log"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func CreateOrUpdate(ctx context.Context, cli client.Client, object client.Object) error {
	resourceVersion, err := getResourceVersion(ctx, cli, object)
	if err != nil {
		return err
	}

	if resourceVersion != "" {
		object.SetResourceVersion(resourceVersion)
		return update(ctx, cli, object)
	} else {
		return create(ctx, cli, object)
	}
}

func getResourceVersion(ctx context.Context, cli client.Client, object client.Object) (string, error) {
	var o = object.DeepCopyObject().(client.Object)
	err := cli.Get(ctx, client.ObjectKeyFromObject(object), o)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return "", nil
		}
		return "", fmt.Errorf("failed to get %s %s: %w", object.GetObjectKind().GroupVersionKind().Kind, object.GetName(), err)
	}
	return o.GetResourceVersion(), nil
}

func create(ctx context.Context, cli client.Client, object client.Object) error {
	log.Printf("Creating %s %s…", object.GetObjectKind().GroupVersionKind().Kind, object.GetName())

	if err := cli.Create(ctx, object); err != nil {
		return fmt.Errorf("failed to create %s %s: %w", object.GetObjectKind().GroupVersionKind().Kind, object.GetName(), err)
	}

	log.Printf("Created %s %s.", object.GetObjectKind().GroupVersionKind().Kind, object.GetName())

	return nil
}

func update(ctx context.Context, cli client.Client, object client.Object) error {
	log.Printf("Updating %s %s…", object.GetObjectKind().GroupVersionKind().Kind, object.GetName())

	if err := cli.Update(ctx, object); err != nil {
		return fmt.Errorf("failed to update %s %s: %w", object.GetObjectKind().GroupVersionKind().Kind, object.GetName(), err)
	}

	log.Printf("Updated %s %s.", object.GetObjectKind().GroupVersionKind().Kind, object.GetName())

	return nil
}

func Delete(ctx context.Context, cli client.Client, object client.Object) error {
	log.Printf("Deleting %s %s…", object.GetObjectKind().GroupVersionKind().Kind, object.GetName())

	if err := cli.Delete(ctx, object); err != nil {
		if apierrors.IsNotFound(err) {
			log.Printf("%s %s not found, skipping deletion.", object.GetObjectKind().GroupVersionKind().Kind, object.GetName())
			return nil
		}
		return fmt.Errorf("failed to delete %s %s: %w", object.GetObjectKind().GroupVersionKind().Kind, object.GetName(), err)
	}

	log.Printf("Deleted %s %s.", object.GetObjectKind().GroupVersionKind().Kind, object.GetName())

	return nil
}

func DeleteAllWithLabel(ctx context.Context, cli client.Client, list client.ObjectList, labelSelector client.MatchingLabels) error {
	gvk := list.GetObjectKind().GroupVersionKind()
	kind := gvk.Kind

	log.Printf("Listing %s resources with labels %v…", kind, labelSelector)

	if err := cli.List(ctx, list, labelSelector); err != nil {
		return fmt.Errorf("failed to list %s resources: %w", kind, err)
	}

	items, err := extractListItems(list)
	if err != nil {
		return err
	}

	if len(items) == 0 {
		log.Printf("No %s resources found with labels %v, skipping deletion.", kind, labelSelector)
		return nil
	}

	log.Printf("Found %d %s resource(s) to delete.", len(items), kind)

	for _, item := range items {
		if err := Delete(ctx, cli, item); err != nil {
			return err
		}
	}

	return nil
}

func extractListItems(list client.ObjectList) ([]client.Object, error) {
	switch v := list.(type) {
	case interface{ GetItems() []client.Object }:
		return v.GetItems(), nil
	default:
		return nil, fmt.Errorf("unsupported list type: %T", list)
	}
}
