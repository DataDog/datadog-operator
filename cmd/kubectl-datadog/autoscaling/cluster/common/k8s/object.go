package k8s

import (
	"context"
	"fmt"
	"log"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// CreateOrUpdate creates the object if missing, otherwise updates it. The
// Update races against any controller actively reconciling the same resource
// (Karpenter mutates its NodePool / EC2NodeClass when a NodeClaim status
// changes), so an Update can fail with a Conflict whenever the controller has
// bumped the resourceVersion since our Get. Re-fetch and retry on Conflict is
// the standard k8s pattern.
func CreateOrUpdate(ctx context.Context, cli client.Client, object client.Object) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
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
	})
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
