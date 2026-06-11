package evict

import (
	"context"
	"errors"
	"fmt"
	"log"
	"reflect"
	"slices"
	"strings"

	"github.com/samber/lo"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/pager"
	"sigs.k8s.io/controller-runtime/pkg/client"

	commonk8s "github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/cluster/common/k8s"
)

// Label keys used to mark PodDisruptionBudgets created by this command. Both
// must be present for cleanup to consider the PDB ours — this makes the
// cleanup safe against accidentally removing a user PDB with a colliding name.
const (
	pdbManagedByLabelKey   = "app.kubernetes.io/managed-by"
	pdbManagedByLabelValue = "kubectl-datadog"
	pdbTempLabelKey        = "autoscaling.datadoghq.com/temporary-pdb"
	pdbTempLabelValue      = "true"
	// pdbNameSuffix is appended to the controller name to form the temp PDB
	// name. Kept short so that long controller names stay under the 63-char
	// DNS label limit after truncation.
	pdbNameSuffix = "-evict-legacy"
)

// ensureTempPDBs scans the pods running on the nodes of every target and
// creates a temporary PodDisruptionBudget (maxUnavailable: 1) for each
// top-level controller (Deployment, StatefulSet, bare ReplicaSet) that does
// not already have one with a matching selector.
//
// The created PDBs carry two labels (managed-by + temporary-pdb) that the
// cleanup step uses to find and delete them, regardless of which process
// created them. ensureTempPDBs itself is idempotent: a PDB created by a
// previous (possibly crashed) run is detected by its labels and left alone.
func ensureTempPDBs(ctx context.Context, clientset kubernetes.Interface, ctrlClient client.Client, targets []Target, dryRun bool) error {
	// Targets across all manager types are merged: the orchestrator blocks
	// on waitEKSNodegroupEmpty before cleaning up the temporary PDBs, so EKS
	// MNG nodes observe the PDBs during their drain too — without that, EKS
	// could disrupt every replica of an otherwise unprotected workload at
	// once when all replicas happen to live on the same node group.
	allNodes := lo.FlatMap(targets, func(t Target, _ int) []string { return t.Nodes })
	nodeSet := lo.SliceToMap(allNodes, func(n string) (string, struct{}) { return n, struct{}{} })
	if len(nodeSet) == 0 {
		return nil
	}

	controllers, err := discoverControllers(ctx, clientset, nodeSet)
	if err != nil {
		return fmt.Errorf("failed to discover controllers: %w", err)
	}
	if len(controllers) == 0 {
		return nil
	}

	// Group controllers by namespace to amortize the per-namespace PDB list.
	byNamespace := make(map[string][]controllerInfo)
	for _, c := range controllers {
		byNamespace[c.Namespace] = append(byNamespace[c.Namespace], c)
	}

	var errs []error
	for ns, ctrls := range byNamespace {
		existing, err := listNamespacePDBs(ctx, clientset, ns)
		if err != nil {
			errs = append(errs, fmt.Errorf("namespace %s: failed to list PDBs: %w", ns, err))
			continue
		}
		for _, c := range ctrls {
			if hasUserPDB(existing, c.Selector) {
				continue
			}
			if err := createTempPDB(ctx, ctrlClient, c, dryRun); err != nil {
				errs = append(errs, fmt.Errorf("controller %s/%s/%s: %w", c.Namespace, c.Kind, c.Name, err))
			}
		}
	}
	return errors.Join(errs...)
}

// cleanupTempPDBs deletes every PodDisruptionBudget cluster-wide that carries
// both temp-PDB labels. Listing by labels (not by a state struct returned from
// ensureTempPDBs) is what makes the command crash-safe: a re-run after a kill
// still finds and removes the orphans left by the previous attempt.
func cleanupTempPDBs(ctx context.Context, ctrlClient client.Client, dryRun bool) error {
	list := &policyv1.PodDisruptionBudgetList{}
	if err := ctrlClient.List(ctx, list, client.MatchingLabels{
		pdbManagedByLabelKey: pdbManagedByLabelValue,
		pdbTempLabelKey:      pdbTempLabelValue,
	}); err != nil {
		return fmt.Errorf("failed to list temporary PDBs: %w", err)
	}
	if len(list.Items) == 0 {
		return nil
	}
	var errs []error
	for i := range list.Items {
		pdb := &list.Items[i]
		if dryRun {
			log.Printf("[dry-run] would delete PDB %s/%s", pdb.Namespace, pdb.Name)
			continue
		}
		if err := commonk8s.Delete(ctx, ctrlClient, pdb); err != nil {
			errs = append(errs, fmt.Errorf("PDB %s/%s: %w", pdb.Namespace, pdb.Name, err))
		}
	}
	return errors.Join(errs...)
}

// controllerKey identifies a top-level controller that owns evictable pods on
// our target nodes. It is the dedup key in the seen map and the identity half
// of controllerInfo.
type controllerKey struct {
	Namespace string
	Kind      string // "Deployment", "StatefulSet", "ReplicaSet"
	Name      string
}

// controllerInfo is a controllerKey plus the controller's pod selector — what a
// PDB would match on.
type controllerInfo struct {
	controllerKey
	Selector *metav1.LabelSelector
}

// discoverControllers lists every Pod cluster-wide once (paginated) and, for
// each Pod scheduled on one of the target nodes, resolves the top-level
// controller. Listing once and filtering client-side avoids the N-API-calls
// problem of doing one List per node, which on a large legacy fleet would
// dominate the command's wall-clock time. The resulting slice contains each
// controller at most once.
func discoverControllers(ctx context.Context, clientset kubernetes.Interface, nodeSet map[string]struct{}) ([]controllerInfo, error) {
	seen := make(map[controllerKey]*metav1.LabelSelector)
	depCache := make(map[client.ObjectKey]*appsv1.Deployment)
	rsCache := make(map[client.ObjectKey]*appsv1.ReplicaSet)
	stsCache := make(map[client.ObjectKey]*appsv1.StatefulSet)

	p := pager.New(func(ctx context.Context, opts metav1.ListOptions) (runtime.Object, error) {
		return clientset.CoreV1().Pods(metav1.NamespaceAll).List(ctx, opts)
	})
	if err := p.EachListItem(ctx, metav1.ListOptions{}, func(obj runtime.Object) error {
		pod := obj.(*corev1.Pod)
		if _, onTarget := nodeSet[pod.Spec.NodeName]; !onTarget {
			return nil
		}
		if shouldSkipEviction(pod) {
			return nil
		}
		info, err := resolveTopLevelController(ctx, clientset, pod, depCache, rsCache, stsCache)
		if err != nil {
			log.Printf("Warning: cannot resolve controller for pod %s/%s: %v", pod.Namespace, pod.Name, err)
			return nil
		}
		if info == nil {
			return nil
		}
		seen[info.controllerKey] = info.Selector
		return nil
	}); err != nil {
		return nil, fmt.Errorf("list pods: %w", err)
	}

	return lo.MapToSlice(seen, func(key controllerKey, selector *metav1.LabelSelector) controllerInfo {
		return controllerInfo{controllerKey: key, Selector: selector}
	}), nil
}

// resolveTopLevelController walks a Pod's owner chain up to the workload
// controller (Deployment > ReplicaSet > Pod, StatefulSet > Pod). Returns nil
// for Pods whose top-level owner is not a stable workload — Jobs (TTL-managed),
// DaemonSets (already skipped at eviction), or bare Pods.
func resolveTopLevelController(
	ctx context.Context,
	clientset kubernetes.Interface,
	pod *corev1.Pod,
	depCache map[client.ObjectKey]*appsv1.Deployment,
	rsCache map[client.ObjectKey]*appsv1.ReplicaSet,
	stsCache map[client.ObjectKey]*appsv1.StatefulSet,
) (*controllerInfo, error) {
	owner := metav1.GetControllerOf(pod)
	if owner == nil {
		return nil, nil
	}
	switch owner.Kind {
	case "ReplicaSet":
		rs, err := getCached(ctx, rsCache, pod.Namespace, owner.Name, clientset.AppsV1().ReplicaSets(pod.Namespace).Get)
		if err != nil {
			return nil, err
		}
		rsOwner := metav1.GetControllerOf(rs)
		if rsOwner != nil && rsOwner.Kind == "Deployment" {
			dep, err := getCached(ctx, depCache, pod.Namespace, rsOwner.Name, clientset.AppsV1().Deployments(pod.Namespace).Get)
			if err != nil {
				return nil, err
			}
			return &controllerInfo{
				controllerKey: controllerKey{Namespace: pod.Namespace, Kind: "Deployment", Name: dep.Name},
				Selector:      dep.Spec.Selector,
			}, nil
		}
		return &controllerInfo{
			controllerKey: controllerKey{Namespace: pod.Namespace, Kind: "ReplicaSet", Name: rs.Name},
			Selector:      rs.Spec.Selector,
		}, nil
	case "StatefulSet":
		sts, err := getCached(ctx, stsCache, pod.Namespace, owner.Name, clientset.AppsV1().StatefulSets(pod.Namespace).Get)
		if err != nil {
			return nil, err
		}
		return &controllerInfo{
			controllerKey: controllerKey{Namespace: pod.Namespace, Kind: "StatefulSet", Name: sts.Name},
			Selector:      sts.Spec.Selector,
		}, nil
	default:
		// DaemonSet (skipped before reaching here), Job (TTL), CronJob,
		// custom controllers — none get a temporary PDB.
		return nil, nil
	}
}

// getCached returns the object identified by (ns, name) from cache, fetching it
// via get — the namespace-bound typed clientset accessor, e.g.
// clientset.AppsV1().ReplicaSets(ns).Get — and populating the cache on a miss.
// T is inferred from the cache's value type, collapsing the per-kind getters
// into a single generic lookup.
func getCached[T any](
	ctx context.Context,
	cache map[client.ObjectKey]*T,
	ns, name string,
	get func(ctx context.Context, name string, opts metav1.GetOptions) (*T, error),
) (*T, error) {
	key := client.ObjectKey{Namespace: ns, Name: name}
	if obj, ok := cache[key]; ok {
		return obj, nil
	}
	obj, err := get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	cache[key] = obj
	return obj, nil
}

// listNamespacePDBs returns every PDB in the namespace. Used to detect
// pre-existing user PDBs covering a controller we'd otherwise PDB-protect.
func listNamespacePDBs(ctx context.Context, clientset kubernetes.Interface, namespace string) ([]policyv1.PodDisruptionBudget, error) {
	list, err := clientset.PolicyV1().PodDisruptionBudgets(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

// hasUserPDB returns true when an existing non-temporary PDB has the same
// selector as the controller's pod selector. This is a conservative
// equality check: a broader user PDB will NOT be detected, and we'll create
// our own. Eviction will then respect the most restrictive of the two,
// preserving the user's intent.
func hasUserPDB(existing []policyv1.PodDisruptionBudget, controllerSelector *metav1.LabelSelector) bool {
	if controllerSelector == nil {
		return false
	}
	return slices.ContainsFunc(existing, func(pdb policyv1.PodDisruptionBudget) bool {
		return !isTemporaryPDB(&pdb) && reflect.DeepEqual(pdb.Spec.Selector, controllerSelector)
	})
}

func isTemporaryPDB(pdb *policyv1.PodDisruptionBudget) bool {
	return pdb.Labels[pdbManagedByLabelKey] == pdbManagedByLabelValue &&
		pdb.Labels[pdbTempLabelKey] == pdbTempLabelValue
}

// createTempPDB writes (or no-ops if our PDB already exists) a temporary
// PodDisruptionBudget with maxUnavailable: 1. Existing PDBs that aren't ours
// at the same name are left alone with a logged warning — that's a name
// collision the user must resolve.
func createTempPDB(ctx context.Context, ctrlClient client.Client, c controllerInfo, dryRun bool) error {
	name := tempPDBName(c.Kind, c.Name)
	if dryRun {
		log.Printf("[dry-run] would create PDB %s/%s (maxUnavailable: 1, selector: %s)", c.Namespace, name, formatSelector(c.Selector))
		return nil
	}

	existing := &policyv1.PodDisruptionBudget{}
	err := ctrlClient.Get(ctx, client.ObjectKey{Namespace: c.Namespace, Name: name}, existing)
	switch {
	case err == nil:
		if !isTemporaryPDB(existing) {
			log.Printf("Warning: PDB %s/%s exists but is not labelled as temporary; leaving it untouched", c.Namespace, name)
			return nil
		}
		// Our PDB from a previous (possibly crashed) run. Leave as-is; the
		// cleanup step will remove it at the end of the current run.
		return nil
	case !apierrors.IsNotFound(err):
		return fmt.Errorf("failed to get PDB %s/%s: %w", c.Namespace, name, err)
	}

	maxUnavailable := intstr.FromInt(1)
	pdb := &policyv1.PodDisruptionBudget{
		TypeMeta: metav1.TypeMeta{APIVersion: "policy/v1", Kind: "PodDisruptionBudget"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: c.Namespace,
			Labels: map[string]string{
				pdbManagedByLabelKey: pdbManagedByLabelValue,
				pdbTempLabelKey:      pdbTempLabelValue,
			},
		},
		Spec: policyv1.PodDisruptionBudgetSpec{
			Selector:       c.Selector.DeepCopy(),
			MaxUnavailable: &maxUnavailable,
		},
	}
	if err := ctrlClient.Create(ctx, pdb); err != nil {
		return fmt.Errorf("failed to create PDB %s/%s: %w", c.Namespace, name, err)
	}
	log.Printf("Created temporary PDB %s/%s for %s/%s (maxUnavailable: 1).", c.Namespace, name, c.Kind, c.Name)
	return nil
}

// tempPDBName builds a DNS-label-safe PDB name. Long controller names are
// truncated so the final name (including the suffix) fits the 63-char limit.
func tempPDBName(kind, controllerName string) string {
	prefix := strings.ToLower(kind) + "-" + controllerName
	if len(prefix)+len(pdbNameSuffix) > validation.DNS1123LabelMaxLength {
		prefix = prefix[:validation.DNS1123LabelMaxLength-len(pdbNameSuffix)]
	}
	return prefix + pdbNameSuffix
}

func formatSelector(s *metav1.LabelSelector) string {
	if s == nil {
		return "<nil>"
	}
	parts := lo.MapToSlice(s.MatchLabels, func(k, v string) string {
		return k + "=" + v
	})
	return strings.Join(parts, ",")
}
