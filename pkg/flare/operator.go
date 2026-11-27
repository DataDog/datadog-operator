// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flare

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Operator collectors and helpers shared by the remote flare and the kubectl plugin.

const (
	// LeaderElectionID is the name of the Operator's leader election Lease.
	LeaderElectionID = "datadog-operator-lock"
	// DefaultHealthProbePort is the port of the Operator health probes.
	DefaultHealthProbePort = "8081"
	// OperatorBinary is the path of the Operator binary in its image.
	OperatorBinary = "/manager"
)

// HealthEndpoints are the health probe endpoints collected in flares.
var HealthEndpoints = []string{"/healthz?verbose", "/readyz?verbose"}

// redactedFlags are flags whose value is never collected: they can hold
// secret backend credentials.
var redactedFlags = map[string]struct{}{
	"secretBackendArgs":   {},
	"secretBackendConfig": {},
}

// collectedEnvPrefixes select the environment variables of the Operator
// process collected by the remote flare.
var collectedEnvPrefixes = []string{"DD_", "WATCH_NAMESPACE", "POD_", "KUBERNETES_SERVICE_", "GOMEMLIMIT", "GOMAXPROCS", "GOGC", "HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY", "http_proxy", "https_proxy", "no_proxy"}

// credentialWords redact environment variables that contain them anywhere in
// their name, such as DD_API_KEY_FILE.
var credentialWords = []string{"KEY", "TOKEN", "SECRET", "PASSWORD", "PASSWD", "CREDENTIAL", "AUTH", "CERT"}

func isSecretFlag(name string) bool {
	_, secret := redactedFlags[name]
	return secret || IsSensitiveKey(name)
}

// RedactFlagValue returns the value of an Operator flag as it's written in
// flares: credentials and secret backend settings are redacted.
func RedactFlagValue(name, value string) string {
	if isSecretFlag(name) && value != "" {
		return redacted
	}
	return value
}

// IsCollectedEnv reports whether an environment variable of the Operator
// process is collected by the remote flare.
func IsCollectedEnv(name string) bool {
	for _, prefix := range collectedEnvPrefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

// RedactEnvValue returns the value of an environment variable as it's
// written in flares: variables whose name looks like a credential are
// redacted.
func RedactEnvValue(name, value string) string {
	if value != "" && isCredentialEnv(name) {
		return redacted
	}
	return value
}

func isCredentialEnv(name string) bool {
	if IsSensitiveKey(name) {
		return true
	}
	upper := strings.ToUpper(name)
	for _, word := range credentialWords {
		if strings.Contains(upper, word) {
			return true
		}
	}
	return false
}

// OperatorContainer returns the Operator container of an Operator pod: the
// one that runs /manager or is named manager, else the first one.
func OperatorContainer(pod *corev1.Pod) *corev1.Container {
	if pod == nil || len(pod.Spec.Containers) == 0 {
		return nil
	}
	for i := range pod.Spec.Containers {
		container := &pod.Spec.Containers[i]
		if container.Name == "manager" || (len(container.Command) > 0 && container.Command[0] == OperatorBinary) {
			return container
		}
	}
	return &pod.Spec.Containers[0]
}

// ContainerFlags returns the flags of a container's command and args, in
// order, as name (without dashes) and value ("true" for a bare flag).
func ContainerFlags(container *corev1.Container) [][2]string {
	if container == nil {
		return nil
	}
	args := append(append([]string{}, container.Command...), container.Args...)
	var flags [][2]string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if !strings.HasPrefix(arg, "-") || arg == "-" || arg == "--" {
			continue
		}
		name, value, hasValue := strings.Cut(strings.TrimLeft(arg, "-"), "=")
		if !hasValue {
			value = "true"
			// Secret flags always take a value, which can start with a
			// dash, such as --secretBackendArgs "--token x".
			if i+1 < len(args) && (!strings.HasPrefix(args[i+1], "-") || isSecretFlag(name)) {
				value = args[i+1]
				i++
			}
		}
		flags = append(flags, [2]string{name, value})
	}
	return flags
}

// ContainerFlag returns the value of the last occurrence of a flag in a
// container's command and args.
func ContainerFlag(container *corev1.Container, name string) (string, bool) {
	value, found := "", false
	for _, flag := range ContainerFlags(container) {
		if flag[0] == name {
			value, found = flag[1], true
		}
	}
	return value, found
}

// OperatorPodConfig is the configuration of an Operator container in its pod
// spec, with credentials redacted.
type OperatorPodConfig struct {
	Source    string            `json:"source"`
	Pod       string            `json:"pod"`
	Container string            `json:"container"`
	Image     string            `json:"image"`
	Command   []string          `json:"command,omitempty"`
	Flags     map[string]string `json:"flags,omitempty"`
	Env       []PodConfigEnv    `json:"env,omitempty"`
	EnvFrom   []string          `json:"envFrom,omitempty"`
}

// PodConfigEnv is an environment variable of the Operator container: its
// value, redacted for credentials, or where it's set from.
type PodConfigEnv struct {
	Name  string `json:"name"`
	Value string `json:"value,omitempty"`
	From  string `json:"from,omitempty"`
}

// OperatorPodConfigCollector collects the Operator configuration from its pod
// spec, for the kubectl plugin.
func OperatorPodConfigCollector() Collector {
	return Collector{Name: "operator/config", Collect: func(_ context.Context, c *Collection) error {
		pod := c.Env.OperatorPod
		container := OperatorContainer(pod)
		if container == nil {
			return errors.New("the Operator pod is unknown")
		}
		config := &OperatorPodConfig{
			Source:    "pod spec (Kubernetes API); flag defaults and in-process settings are not visible from outside the Operator",
			Pod:       pod.Name,
			Container: container.Name,
			Image:     container.Image,
			Flags:     map[string]string{},
		}
		for _, arg := range container.Command {
			if !strings.HasPrefix(arg, "-") {
				config.Command = append(config.Command, arg)
			}
		}
		for _, flag := range ContainerFlags(container) {
			config.Flags[flag[0]] = RedactFlagValue(flag[0], flag[1])
		}
		for _, env := range container.Env {
			config.Env = append(config.Env, podConfigEnv(env))
		}
		for _, from := range container.EnvFrom {
			switch {
			case from.SecretRef != nil:
				config.EnvFrom = append(config.EnvFrom, fmt.Sprintf("%sSecret %s", from.Prefix, from.SecretRef.Name))
			case from.ConfigMapRef != nil:
				config.EnvFrom = append(config.EnvFrom, fmt.Sprintf("%sConfigMap %s", from.Prefix, from.ConfigMapRef.Name))
			}
		}
		return c.AddYAML("operator/config.yaml", config)
	}}
}

func podConfigEnv(env corev1.EnvVar) PodConfigEnv {
	entry := PodConfigEnv{Name: env.Name}
	from := env.ValueFrom
	switch {
	case from == nil:
		entry.Value = RedactEnvValue(env.Name, env.Value)
	case from.SecretKeyRef != nil:
		entry.From = fmt.Sprintf("Secret %s, key %s", from.SecretKeyRef.Name, from.SecretKeyRef.Key)
	case from.ConfigMapKeyRef != nil:
		entry.From = fmt.Sprintf("ConfigMap %s, key %s", from.ConfigMapKeyRef.Name, from.ConfigMapKeyRef.Key)
	case from.FieldRef != nil:
		entry.From = "field " + from.FieldRef.FieldPath
	case from.ResourceFieldRef != nil:
		entry.From = "resource " + from.ResourceFieldRef.Resource
	default:
		entry.From = "another source"
	}
	return entry
}

// GetLease returns the spec of the Operator's leader election Lease.
func GetLease(ctx context.Context, reader client.Reader, namespace, name string) (*coordinationv1.LeaseSpec, error) {
	lease := &coordinationv1.Lease{}
	if err := reader.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, lease); err != nil {
		return nil, fmt.Errorf("unable to get the leader election lease %s/%s: %w", namespace, name, err)
	}
	return &lease.Spec, nil
}

// LeaderElectionCollector collects the Operator's leader election Lease,
// and the pod that holds it, from the Kubernetes API.
func LeaderElectionCollector(leaseName string) Collector {
	return Collector{Name: "operator/leader-election", Collect: func(ctx context.Context, c *Collection) error {
		if c.Env.Reader == nil {
			return errors.New("no Kubernetes reader")
		}
		info := map[string]any{"id": leaseName}
		if c.Env.OperatorPod != nil {
			info["leaderPod"] = c.Env.OperatorPod.Name
		}
		spec, err := GetLease(ctx, c.Env.Reader, c.Env.OperatorNamespace, leaseName)
		if err != nil {
			// Older Operators use a ConfigMap lock, collected with the
			// other ConfigMaps.
			c.Warn(err)
		} else {
			info["lease"] = spec
		}
		return c.AddYAML("operator/leader-election.yaml", info)
	}}
}

// HealthGetter reads a health probe endpoint, such as /readyz?verbose. It
// returns the HTTP status even when the endpoint reports a failure.
type HealthGetter func(ctx context.Context, endpoint string) (status string, body []byte, err error)

// CollectHealthEndpoints writes the Operator health probe endpoints to
// operator/health.txt.
func CollectHealthEndpoints(ctx context.Context, c *Collection, get HealthGetter) error {
	failures := 0
	var out bytes.Buffer
	for _, endpoint := range HealthEndpoints {
		status, body, err := get(ctx, endpoint)
		if err != nil {
			failures++
			c.Warn(fmt.Errorf("unable to read %s: %w", endpoint, err))
			continue
		}
		if strings.HasPrefix(status, "404") {
			// controller-runtime only serves the endpoints that have
			// checks: the Operator has no readiness checks.
			fmt.Fprintf(&out, "GET %s: %s (no checks are registered for this endpoint)\n\n", endpoint, status)
			continue
		}
		fmt.Fprintf(&out, "GET %s: %s\n%s\n\n", endpoint, status, body)
	}
	if failures == len(HealthEndpoints) {
		return errors.New("unable to read the health probes")
	}
	return c.AddText("operator/health.txt", out.Bytes())
}

// PodHealth is the health of a pod as the Kubernetes API shows it.
type PodHealth struct {
	Pod        string                  `json:"pod"`
	Conditions []corev1.PodCondition   `json:"conditions,omitempty"`
	Containers []ContainerHealthStatus `json:"containers,omitempty"`
}

// ContainerHealthStatus is the probe status of a container.
type ContainerHealthStatus struct {
	Name           string                `json:"name"`
	Ready          bool                  `json:"ready"`
	Started        *bool                 `json:"started,omitempty"`
	RestartCount   int32                 `json:"restartCount"`
	State          corev1.ContainerState `json:"state"`
	LastState      corev1.ContainerState `json:"lastState,omitzero"`
	ReadinessProbe *corev1.Probe         `json:"readinessProbe,omitempty"`
	LivenessProbe  *corev1.Probe         `json:"livenessProbe,omitempty"`
	StartupProbe   *corev1.Probe         `json:"startupProbe,omitempty"`
}

// SummarizePodHealth returns the health of a pod.
func SummarizePodHealth(pod *corev1.Pod) PodHealth {
	health := PodHealth{Pod: pod.Name, Conditions: pod.Status.Conditions}
	probes := map[string]*corev1.Container{}
	for i := range pod.Spec.Containers {
		probes[pod.Spec.Containers[i].Name] = &pod.Spec.Containers[i]
	}
	for _, status := range pod.Status.ContainerStatuses {
		container := ContainerHealthStatus{
			Name:         status.Name,
			Ready:        status.Ready,
			Started:      status.Started,
			RestartCount: status.RestartCount,
			State:        status.State,
			LastState:    status.LastTerminationState,
		}
		if spec := probes[status.Name]; spec != nil {
			container.ReadinessProbe = spec.ReadinessProbe
			container.LivenessProbe = spec.LivenessProbe
			container.StartupProbe = spec.StartupProbe
		}
		health.Containers = append(health.Containers, container)
	}
	return health
}

// HealthProbePort returns the port of the Operator health probes: the HTTP
// port of its readiness or liveness probe, else 8081.
func HealthProbePort(pod *corev1.Pod) string {
	container := OperatorContainer(pod)
	if container == nil {
		return DefaultHealthProbePort
	}
	for _, probe := range []*corev1.Probe{container.ReadinessProbe, container.LivenessProbe, container.StartupProbe} {
		if probe == nil || probe.HTTPGet == nil {
			continue
		}
		if port := resolvePort(container, probe.HTTPGet.Port); port != "" {
			return port
		}
	}
	return DefaultHealthProbePort
}

func resolvePort(container *corev1.Container, port intstr.IntOrString) string {
	if port.Type == intstr.Int {
		if port.IntVal > 0 {
			return strconv.Itoa(int(port.IntVal))
		}
		return ""
	}
	for _, p := range container.Ports {
		if p.Name == port.StrVal {
			return strconv.Itoa(int(p.ContainerPort))
		}
	}
	return ""
}
