// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package metadata

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"sync"
	"time"

	"github.com/go-logr/logr"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/pkg/config"
)

const (
	crdHeartbeatInterval = 10 * time.Minute
	crdNumWorkers        = 3
)

// crdSnapshot holds the last-sent CRDInstance and its content hash, keyed by EncodeKey().
type crdSnapshot struct {
	instance CRDInstance
	hash     string
}

type CRDMetadataForwarder struct {
	*SharedMetadata

	mgr         manager.Manager
	enabledCRDs EnabledCRDKindsConfig

	runner *InformerWorkQueue

	// crdSnapshots is keyed by "<Kind>/<namespace>/<name>" and stores *crdSnapshot.
	crdSnapshots sync.Map
}

type CRDMetadataPayload struct {
	UUID      string      `json:"uuid"`
	Timestamp int64       `json:"timestamp"`
	ClusterID string      `json:"cluster_id"`
	Metadata  CRDMetadata `json:"datadog_operator_crd_metadata"`
}

type CRDMetadata struct {
	// Shared
	OperatorVersion   string `json:"operator_version"`
	KubernetesVersion string `json:"kubernetes_version"`
	ClusterID         string `json:"cluster_id"`

	CRDKind            string `json:"crd_kind"`
	CRDName            string `json:"crd_name"`
	CRDNamespace       string `json:"crd_namespace"`
	CRDAPIVersion      string `json:"crd_api_version"`
	CRDUID             string `json:"crd_uid"`
	CRDSpecFull        string `json:"crd_spec_full"`
	CRDLabelsJSON      string `json:"crd_labels,omitempty"`
	CRDAnnotationsJSON string `json:"crd_annotations,omitempty"`
}

type CRDInstance struct {
	Kind        string            `json:"kind"`
	Name        string            `json:"name"`
	Namespace   string            `json:"namespace"`
	APIVersion  string            `json:"api_version"`
	UID         string            `json:"uid"`
	Spec        any               `json:"spec"`
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

// EnabledCRDsConfig specifies which CRD kinds are enabled for metadata collection
type EnabledCRDKindsConfig struct {
	DatadogAgentEnabled         bool
	DatadogAgentInternalEnabled bool
	DatadogAgentProfileEnabled  bool
}

// NewCRDMetadataForwarder creates a new CRD metadata forwarder. The forwarder must be
// registered with the manager via mgr.Add(...) by the caller.
func NewCRDMetadataForwarder(
	logger logr.Logger,
	mgr manager.Manager,
	kubernetesVersion string,
	operatorVersion string,
	credsManager *config.CredentialManager,
	cfg EnabledCRDKindsConfig,
) *CRDMetadataForwarder {
	forwarderLogger := logger.WithName("crd")
	cmf := &CRDMetadataForwarder{
		SharedMetadata: NewSharedMetadata(forwarderLogger, mgr.GetClient(), kubernetesVersion, operatorVersion, credsManager),
		mgr:            mgr,
		enabledCRDs:    cfg,
	}
	cmf.runner = NewInformerWorkQueue(
		forwarderLogger,
		mgr,
		crdNumWorkers,
		crdHeartbeatInterval,
		cmf.processKey,
		cmf.handleDelete,
		cmf.heartbeat,
	)
	return cmf
}

// Start implements manager.Runnable. Registers informer watches for each enabled
// CRD kind and then runs the InformerWorkQueue until ctx is cancelled.
func (cmf *CRDMetadataForwarder) Start(ctx context.Context) error {
	if cmf.enabledCRDs.DatadogAgentEnabled {
		cmf.runner.AddWatch(ctx, WatchTarget{
			Object:   &v2alpha1.DatadogAgent{},
			Group:    "datadoghq.com",
			Resource: "datadogagents",
			Kind:     "DatadogAgent",
		})
	}
	if cmf.enabledCRDs.DatadogAgentInternalEnabled {
		cmf.runner.AddWatch(ctx, WatchTarget{
			Object:   &v1alpha1.DatadogAgentInternal{},
			Group:    "datadoghq.com",
			Resource: "datadogagentinternals",
			Kind:     "DatadogAgentInternal",
		})
	}
	if cmf.enabledCRDs.DatadogAgentProfileEnabled {
		cmf.runner.AddWatch(ctx, WatchTarget{
			Object:   &v1alpha1.DatadogAgentProfile{},
			Group:    "datadoghq.com",
			Resource: "datadogagentprofiles",
			Kind:     "DatadogAgentProfile",
		})
	}

	go cmf.runner.Run(ctx)
	return nil
}

// NeedLeaderElection implements manager.LeaderElectionRunnable. CRD metadata sends
// must be deduplicated across operator replicas.
func (cmf *CRDMetadataForwarder) NeedLeaderElection() bool {
	return true
}

func (cmf *CRDMetadataForwarder) sendCRDMetadata(ctx context.Context, crdInstance CRDInstance) error {
	clusterUID, err := cmf.GetOrCreateClusterUID(ctx)
	if err != nil {
		return fmt.Errorf("error getting cluster UID: %w", err)
	}

	payload := cmf.buildPayload(clusterUID, crdInstance)

	req, err := cmf.createRequest(payload)
	if err != nil {
		return fmt.Errorf("error creating request: %w", err)
	}

	req = req.WithContext(ctx)

	resp, err := cmf.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("error sending metadata request: %w", err)
	}

	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read metadata response body: %w", err)
	}

	cmf.logger.V(2).Info("Sent metadata",
		"statusCode", resp.StatusCode,
		"body", string(body),
		"kind", crdInstance.Kind,
		"name", crdInstance.Name)

	return nil
}

// marshalToJSON marshals data to JSON, returning empty object on error
func (cmf *CRDMetadataForwarder) marshalToJSON(data any, fieldName string, crdInstance CRDInstance) []byte {
	if data == nil {
		return nil
	}

	jsonBytes, err := json.Marshal(data)
	if err != nil {
		cmf.logger.V(1).Info("Error marshaling CRD field to JSON", "error", err,
			"field", fieldName,
			"kind", crdInstance.Kind,
			"name", crdInstance.Name)
		return []byte("{}")
	}
	return jsonBytes
}

func (cmf *CRDMetadataForwarder) buildPayload(clusterUID string, crdInstance CRDInstance) []byte {
	now := time.Now().Unix()

	specJSON := cmf.marshalToJSON(crdInstance.Spec, "spec", crdInstance)
	labelsJSON := cmf.marshalToJSON(crdInstance.Labels, "labels", crdInstance)
	annotationsJSON := cmf.marshalToJSON(crdInstance.Annotations, "annotations", crdInstance)

	crdMetadata := CRDMetadata{
		OperatorVersion:    cmf.operatorVersion,
		KubernetesVersion:  cmf.kubernetesVersion,
		ClusterID:          clusterUID,
		CRDKind:            crdInstance.Kind,
		CRDName:            crdInstance.Name,
		CRDNamespace:       crdInstance.Namespace,
		CRDAPIVersion:      crdInstance.APIVersion,
		CRDUID:             crdInstance.UID,
		CRDSpecFull:        string(specJSON),
		CRDLabelsJSON:      string(labelsJSON),
		CRDAnnotationsJSON: string(annotationsJSON),
	}

	payload := CRDMetadataPayload{
		UUID:      clusterUID,
		Timestamp: now,
		ClusterID: clusterUID,
		Metadata:  crdMetadata,
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		cmf.logger.V(1).Info("Error marshaling payload to json", "error", err)
	}

	return jsonPayload
}

// hashCRD computes a SHA256 hash of the CRD spec, labels, and annotations for change detection
func hashCRD(crd CRDInstance) (string, error) {
	// Hash spec, labels, and annotations together
	hashable := struct {
		Spec        any               `json:"spec"`
		Labels      map[string]string `json:"labels,omitempty"`
		Annotations map[string]string `json:"annotations,omitempty"`
	}{
		Spec:        crd.Spec,
		Labels:      crd.Labels,
		Annotations: crd.Annotations,
	}

	hashableJSON, err := json.Marshal(hashable)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(hashableJSON)
	return fmt.Sprintf("%x", hash), nil
}

// processKey fetches the CRD by kind/namespace/name, builds a CRDInstance, hashes,
// compares against the last snapshot, and sends if new or changed.
func (cmf *CRDMetadataForwarder) processKey(ctx context.Context, kind, namespace, name string) error {
	instance, err := cmf.fetchCRDInstance(ctx, kind, namespace, name)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to fetch %s/%s/%s: %w", kind, namespace, name, err)
	}
	if instance == nil {
		return nil
	}

	newHash, err := hashCRD(*instance)
	if err != nil {
		cmf.logger.V(1).Info("Failed to hash CRD", "error", err,
			"kind", kind, "namespace", namespace, "name", name)
		return nil
	}

	key := EncodeKey(kind, namespace, name)
	if existing, ok := cmf.crdSnapshots.Load(key); ok {
		if existing.(*crdSnapshot).hash == newHash {
			return nil
		}
	}

	// Store the new snapshot before sending so a concurrent heartbeat tick can't
	// observe and re-send the stale (previous) snapshot after this send completes.
	// On send failure, mark the hash empty so the workqueue retry detects a
	// mismatch and re-sends instead of being short-circuited by the cache.
	cmf.crdSnapshots.Store(key, &crdSnapshot{instance: *instance, hash: newHash})
	if err := cmf.sendCRDMetadata(ctx, *instance); err != nil {
		cmf.crdSnapshots.Store(key, &crdSnapshot{instance: *instance, hash: ""})
		cmf.logger.V(1).Info("Failed to send CRD metadata", "error", err,
			"kind", kind, "namespace", namespace, "name", name)
		return err
	}
	return nil
}

// fetchCRDInstance gets the typed object from the cache and converts to CRDInstance.
// Returns (nil, nil) if the kind is unknown.
func (cmf *CRDMetadataForwarder) fetchCRDInstance(ctx context.Context, kind, namespace, name string) (*CRDInstance, error) {
	switch kind {
	case "DatadogAgent":
		dda := &v2alpha1.DatadogAgent{}
		if err := cmf.k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, dda); err != nil {
			return nil, err
		}
		return crdInstanceFromDDA(dda), nil
	case "DatadogAgentInternal":
		ddai := &v1alpha1.DatadogAgentInternal{}
		if err := cmf.k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, ddai); err != nil {
			return nil, err
		}
		return crdInstanceFromDDAI(ddai), nil
	case "DatadogAgentProfile":
		dap := &v1alpha1.DatadogAgentProfile{}
		if err := cmf.k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, dap); err != nil {
			return nil, err
		}
		return crdInstanceFromDAP(dap), nil
	default:
		cmf.logger.V(1).Info("Unknown CRD kind", "kind", kind)
		return nil, nil
	}
}

func cleanedAnnotations(in map[string]string) map[string]string {
	out := maps.Clone(in)
	delete(out, "kubectl.kubernetes.io/last-applied-configuration")
	return out
}

func crdInstanceFromDDA(dda *v2alpha1.DatadogAgent) *CRDInstance {
	return &CRDInstance{
		Kind:        "DatadogAgent",
		Name:        dda.Name,
		Namespace:   dda.Namespace,
		APIVersion:  dda.APIVersion,
		UID:         string(dda.UID),
		Spec:        dda.Spec,
		Labels:      dda.Labels,
		Annotations: cleanedAnnotations(dda.Annotations),
	}
}

func crdInstanceFromDDAI(ddai *v1alpha1.DatadogAgentInternal) *CRDInstance {
	return &CRDInstance{
		Kind:        "DatadogAgentInternal",
		Name:        ddai.Name,
		Namespace:   ddai.Namespace,
		APIVersion:  ddai.APIVersion,
		UID:         string(ddai.UID),
		Spec:        ddai.Spec,
		Labels:      ddai.Labels,
		Annotations: cleanedAnnotations(ddai.Annotations),
	}
}

func crdInstanceFromDAP(dap *v1alpha1.DatadogAgentProfile) *CRDInstance {
	return &CRDInstance{
		Kind:        "DatadogAgentProfile",
		Name:        dap.Name,
		Namespace:   dap.Namespace,
		APIVersion:  dap.APIVersion,
		UID:         string(dap.UID),
		Spec:        dap.Spec,
		Labels:      dap.Labels,
		Annotations: cleanedAnnotations(dap.Annotations),
	}
}

// handleDelete is the DeleteFunc callback for InformerWorkQueue.
func (cmf *CRDMetadataForwarder) handleDelete(kind, namespace, name string) {
	key := EncodeKey(kind, namespace, name)
	cmf.crdSnapshots.Delete(key)
	cmf.logger.V(1).Info("Removed deleted CRD from snapshot store",
		"kind", kind, "namespace", namespace, "name", name)
}

// heartbeat is the HeartbeatFunc callback for InformerWorkQueue.
func (cmf *CRDMetadataForwarder) heartbeat(ctx context.Context) {
	cmf.crdSnapshots.Range(func(key, value any) bool {
		snap := value.(*crdSnapshot)
		if err := cmf.sendCRDMetadata(ctx, snap.instance); err != nil {
			cmf.logger.V(1).Info("Failed to send CRD metadata during heartbeat",
				"key", key, "error", err)
		}
		return true
	})
}
