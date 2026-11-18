// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package resources

import (
	"encoding/json"
	"fmt"
	"math"
	"net"
	"slices"
	"strconv"
	"strings"
	"unicode"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/yaml"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	byocimage "github.com/DataDog/datadog-operator/internal/controller/datadogbyoccluster/image"
	controllerutils "github.com/DataDog/datadog-operator/internal/controller/utils"
)

// maxWorkerNameLength keeps the worker StatefulSet name short enough for the StatefulSet controller to set the
// controller-revision-hash label, whose value is <statefulset>-<hash>, within the 63-character label value limit.
const maxWorkerNameLength = 52

const (
	// Pomsky's ingest_api defaults (quickwit-config/src/node_config/mod.rs).
	pomskyDefaultMaxBodySize             = 10 << 20
	pomskyDefaultMaxDecompressedBodySize = 256 << 20
	// The Worker ignores a CloudPrem max payload below the 5MB limit of the Datadog intake.
	pipelineMinMaxPayloadBytes = 5_000_000
)

// BuildObservabilityPipelinesWorker builds a named child worker from a defaulted pipeline and shared BYOC settings.
// The cluster and pipeline are not modified, and the returned worker does not share mutable settings with them.
func BuildObservabilityPipelinesWorker(cluster *datadoghqv1alpha1.DatadogBYOCCluster, pipeline *datadoghqv1alpha1.DatadogBYOCClusterPipelineSpec, image byocimage.ResolvedImage) (*datadoghqv1alpha1.DatadogObservabilityPipelinesWorker, error) {
	workerName := ComponentResourceName(cluster.Name, PipelineComponentName+"-"+pipeline.Name)
	// The worker name is also used for its Service.
	if problems := validation.IsDNS1035Label(workerName); len(problems) > 0 {
		return nil, fmt.Errorf("invalid worker resource name %q: %s", workerName, strings.Join(problems, ", "))
	}
	if len(workerName) > maxWorkerNameLength {
		return nil, fmt.Errorf("invalid worker resource name %q: must be no more than %d characters", workerName, maxWorkerNameLength)
	}
	component := pipeline.DatadogBYOCClusterPipelineComponentSpec.DeepCopy()
	if err := applyGlobalPipelineSettings(cluster, component); err != nil {
		return nil, err
	}

	workerImage := datadoghqv1alpha1.DatadogBYOCImageSpec{
		Repository:       new(image.Repository),
		PullPolicy:       new(image.GetImagePullPolicy()),
		ImagePullSecrets: slices.Clone(image.ImagePullSecrets),
	}
	if image.Digest != "" {
		workerImage.Digest = new(image.Digest)
	} else {
		workerImage.Tag = new(image.Tag)
	}
	resolvedImage := datadoghqv1alpha1.DatadogObservabilityPipelinesWorkerImageSpec(workerImage)

	site := ptr.Deref(cluster.Spec.Datadog.Site, "datadoghq.com")
	apiKeySecretRef := cluster.Spec.Datadog.APIKeySecretRef
	if pipeline.Datadog != nil && pipeline.Datadog.APIKeySecretRef != nil {
		apiKeySecretRef = pipeline.Datadog.APIKeySecretRef
	}
	return &datadoghqv1alpha1.DatadogObservabilityPipelinesWorker{
		ObjectMeta: metav1.ObjectMeta{
			Name:      workerName,
			Namespace: cluster.Namespace,
		},
		Spec: datadoghqv1alpha1.DatadogObservabilityPipelinesWorkerSpec{
			DatadogBYOCClusterPipelineComponentSpec: *component.DeepCopy(),
			Datadog: &datadoghqv1alpha1.DatadogObservabilityPipelinesWorkerDatadogSpec{
				Site:            new(site),
				APIKeySecretRef: apiKeySecretRef.DeepCopy(),
			},
			Image:    &resolvedImage,
			Identity: pipeline.Identity.DeepCopy(),
		},
	}, nil
}

func applyGlobalPipelineSettings(cluster *datadoghqv1alpha1.DatadogBYOCCluster, pipeline *datadoghqv1alpha1.DatadogBYOCClusterPipelineComponentSpec) error {
	scheme, err := indexerEndpointScheme(cluster)
	if err != nil {
		return err
	}

	global := cluster.Spec.Global
	component := &pipeline.DatadogBYOCClusterComponentSpec

	component.Labels = labels(cluster, componentLabel(PipelineComponentName), component.Labels)
	component.Annotations = annotations(cluster, component.Annotations)
	component.Env = controllerutils.MergeEnv(global.Env, component.Env)
	// Logs (default) and traces use different OPW destinations; traces needs the Pomsky BYOC traces route as a path.
	traces := cluster.Spec.Signal != nil && *cluster.Spec.Signal == datadoghqv1alpha1.DatadogBYOCClusterSignalTraces
	destinationEnv := envPipelineDestinationEndpoint
	endpoint := scheme + "://" + net.JoinHostPort(ComponentResourceName(cluster.Name, IndexerComponentName), strconv.Itoa(int(restPort)))
	if traces {
		destinationEnv = envPipelineDestinationTracesEndpoint
		endpoint += byocTracesPath
	}
	// A variable set by the user keeps its position, so that it can reference the variables defined before it.
	if !hasEnv(component.Env, destinationEnv) {
		component.Env = append(component.Env, corev1.EnvVar{Name: destinationEnv, Value: endpoint})
	}
	if !traces && !hasEnv(component.Env, envPipelineDestinationMaxPayloadBytes) {
		maxPayloadBytes, err := pipelineMaxPayloadBytes(cluster)
		if err != nil {
			return err
		}
		// OPW ignores a lower value, so the Worker keeps its default.
		if maxPayloadBytes >= pipelineMinMaxPayloadBytes {
			component.Env = append(component.Env, corev1.EnvVar{Name: envPipelineDestinationMaxPayloadBytes, Value: strconv.FormatInt(maxPayloadBytes, 10)})
		}
	}
	component.EnvFrom = slices.Concat(global.EnvFrom, component.EnvFrom)
	component.Volumes = controllerutils.MergeVolumes(global.Volumes, component.Volumes)
	component.VolumeMounts = controllerutils.MergeVolumeMounts(global.VolumeMounts, component.VolumeMounts)
	component.Tolerations = slices.Concat(global.Tolerations, component.Tolerations)
	component.TopologySpreadConstraints = controllerutils.MergeTopologySpreadConstraints(global.TopologySpreadConstraints, component.TopologySpreadConstraints)
	if global.Affinity != nil {
		var err error
		component.Affinity, err = controllerutils.MergeAffinity(global.Affinity, component.Affinity)
		if err != nil {
			return fmt.Errorf("merge pipeline affinity: %w", err)
		}
	}
	if component.PodDisruptionBudget == nil {
		component.PodDisruptionBudget = global.PodDisruptionBudget
		if component.PodDisruptionBudget == nil {
			component.PodDisruptionBudget = &datadoghqv1alpha1.DatadogBYOCClusterPodDisruptionBudgetSpec{
				MaxUnavailable: new(intstr.FromInt32(1)),
			}
		}
	}
	return nil
}

func hasEnv(envs []corev1.EnvVar, name string) bool {
	return slices.ContainsFunc(envs, func(env corev1.EnvVar) bool { return env.Name == name })
}

// pipelineMaxPayloadBytes returns the largest uncompressed payload the Worker can send to the indexer.
// Pomsky answers a body above ingest_api.max_body_size on the wire, or above max_decompressed_body_size once
// decompressed, with a 413 that the Worker does not retry, so the whole batch is lost. The Worker limits the
// uncompressed payload, and compressing incompressible data adds a few bytes, so 1% is kept as headroom.
func pipelineMaxPayloadBytes(cluster *datadoghqv1alpha1.DatadogBYOCCluster) (int64, error) {
	maxBodySize := int64(pomskyDefaultMaxBodySize)
	maxDecompressedBodySize := int64(pomskyDefaultMaxDecompressedBodySize)
	if cluster.Spec.NodeConfigOverrides != nil && len(cluster.Spec.NodeConfigOverrides.Raw) != 0 {
		var nodeConfig struct {
			IngestAPI struct {
				MaxBodySize             json.RawMessage `json:"max_body_size"`
				ContentLengthLimit      json.RawMessage `json:"content_length_limit"`
				MaxDecompressedBodySize json.RawMessage `json:"max_decompressed_body_size"`
			} `json:"ingest_api"`
		}
		if err := yaml.Unmarshal(cluster.Spec.NodeConfigOverrides.Raw, &nodeConfig); err != nil {
			return 0, fmt.Errorf("decode spec.nodeConfigOverrides: %w", err)
		}
		// content_length_limit is the serde alias of max_body_size, so Pomsky rejects setting both.
		for _, setting := range []struct {
			key    string
			value  json.RawMessage
			target *int64
		}{
			{"max_body_size", nodeConfig.IngestAPI.MaxBodySize, &maxBodySize},
			{"content_length_limit", nodeConfig.IngestAPI.ContentLengthLimit, &maxBodySize},
			{"max_decompressed_body_size", nodeConfig.IngestAPI.MaxDecompressedBodySize, &maxDecompressedBodySize},
		} {
			if setting.value == nil {
				continue
			}
			size, err := parseByteSize(setting.value)
			if err != nil {
				return 0, fmt.Errorf("decode spec.nodeConfigOverrides ingest_api.%s: %w", setting.key, err)
			}
			*setting.target = size
		}
	}
	limit := min(maxBodySize, maxDecompressedBodySize)
	return limit - limit/100, nil
}

// byteSizeUnits are the units of the bytesize crate that Pomsky parses its byte sizes with.
var byteSizeUnits = map[string]float64{
	"b": 1,
	"k": 1e3, "kb": 1e3, "m": 1e6, "mb": 1e6, "g": 1e9, "gb": 1e9, "t": 1e12, "tb": 1e12, "p": 1e15, "pb": 1e15, "e": 1e18, "eb": 1e18,
	"ki": 1 << 10, "kib": 1 << 10, "mi": 1 << 20, "mib": 1 << 20, "gi": 1 << 30, "gib": 1 << 30, "ti": 1 << 40, "tib": 1 << 40, "pi": 1 << 50, "pib": 1 << 50, "ei": 1 << 60, "eib": 1 << 60,
}

// parseByteSize parses a byte size like the bytesize crate: an unsigned integer count of bytes, or a string that
// holds either one or a decimal number followed by a case-insensitive unit, such as "20M" or "1.5 GiB".
func parseByteSize(value json.RawMessage) (int64, error) {
	if size, err := strconv.ParseUint(string(value), 10, 63); err == nil {
		return int64(size), nil
	}
	var s string
	if err := json.Unmarshal(value, &s); err != nil {
		return 0, fmt.Errorf("%s is not a byte size", value)
	}
	if size, err := strconv.ParseUint(s, 10, 63); err == nil {
		return int64(size), nil
	}
	numberLength := strings.IndexFunc(s, func(r rune) bool { return (r < '0' || r > '9') && r != '.' })
	if numberLength < 0 {
		numberLength = len(s)
	}
	number, err := strconv.ParseFloat(s[:numberLength], 64)
	if err != nil {
		return 0, fmt.Errorf("%q is not a byte size", s)
	}
	unit, found := byteSizeUnits[strings.ToLower(strings.TrimLeftFunc(s[numberLength:], unicode.IsSpace))]
	if !found {
		return 0, fmt.Errorf("%q is not a byte size", s)
	}
	if size := number * unit; size < math.MaxInt64 {
		return int64(size), nil
	}
	return math.MaxInt64, nil
}

func indexerEndpointScheme(cluster *datadoghqv1alpha1.DatadogBYOCCluster) (string, error) {
	const (
		httpScheme  = "http"
		httpsScheme = "https"
	)

	if cluster.Spec.NodeConfigOverrides == nil || len(cluster.Spec.NodeConfigOverrides.Raw) == 0 {
		return httpScheme, nil
	}

	var nodeConfig struct {
		REST struct {
			TLS map[string]any `json:"tls"`
		} `json:"rest"`
	}
	if err := yaml.Unmarshal(cluster.Spec.NodeConfigOverrides.Raw, &nodeConfig); err != nil {
		return "", fmt.Errorf("decode spec.nodeConfigOverrides: %w", err)
	}
	if nodeConfig.REST.TLS != nil {
		return httpsScheme, nil
	}
	return httpScheme, nil
}
