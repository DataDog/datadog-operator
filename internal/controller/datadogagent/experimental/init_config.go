// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package experimental

import (
	"context"
	"fmt"
	"path"
	"slices"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
)

// ExperimentalWithoutInitConfigSubkey enables the direct-file-mount experiment.
// It requires an Agent image that replaces the init-config scripts' runtime behavior.
const ExperimentalWithoutInitConfigSubkey = "without-init-config"

// ApplyWithoutInitConfig replaces init-config's file copies with per-file mounts.
// Validation runs on a copy so an invalid configuration cannot partially change the template.
func ApplyWithoutInitConfig(ctx context.Context, reader client.Reader, ddai *v1alpha1.DatadogAgentInternal, template *corev1.PodTemplateSpec) error {
	switch getExperimentalAnnotation(ddai, ExperimentalWithoutInitConfigSubkey) {
	case "", "false":
		return nil
	case "true":
	default:
		return fmt.Errorf("%s must be true or false", getExperimentalAnnotationKey(ExperimentalWithoutInitConfigSubkey))
	}
	updated := template.DeepCopy()
	if err := mountWithoutInitConfig(ctx, reader, ddai, &updated.Spec); err != nil {
		return fmt.Errorf("without-init-config: %w", err)
	}
	*template = *updated
	return nil
}

func mountWithoutInitConfig(ctx context.Context, reader client.Reader, ddai *v1alpha1.DatadogAgentInternal, spec *corev1.PodSpec) error {
	if spec.OS != nil && spec.OS.Name != corev1.Linux {
		return fmt.Errorf("requires a Linux node Agent pod")
	}
	if !slices.ContainsFunc(spec.InitContainers, func(c corev1.Container) bool { return c.Name == "init-volume" }) {
		return fmt.Errorf("requires init-volume and the shared configuration directory")
	}
	var mounts []corev1.VolumeMount
	override := ddai.Spec.Override[v2alpha1.NodeAgentComponentName]
	for _, volume := range spec.Volumes {
		var directory, extension string
		var custom *v2alpha1.MultiCustomConfig
		switch volume.Name {
		case common.ConfdVolumeName:
			directory, extension = "conf.d", ".yaml"
			if override != nil {
				custom = override.ExtraConfd
			}
		case common.ChecksdVolumeName:
			directory, extension = "checks.d", ".py"
			if override != nil {
				custom = override.ExtraChecksd
			}
		default:
			continue
		}
		if volume.EmptyDir != nil {
			continue
		}
		if volume.ConfigMap == nil {
			return fmt.Errorf("volume %s must use a ConfigMap", volume.Name)
		}
		items, err := configMapItems(ctx, reader, ddai.Namespace, volume, custom)
		if err != nil {
			return err
		}
		paths := map[string]bool{}
		for _, item := range items {
			if !safeConfigPath(item.Path) || paths[item.Path] {
				return fmt.Errorf("volume %s has an unsafe or duplicate file path %q", volume.Name, item.Path)
			}
			paths[item.Path] = true
			if !strings.HasSuffix(item.Path, extension) {
				continue
			}
			mounts = append(mounts, corev1.VolumeMount{
				Name: volume.Name, MountPath: path.Join(common.ConfigVolumePath, directory, item.Path),
				SubPath: item.Path, ReadOnly: true,
			})
		}
	}
	consumers := 0
	for i := range spec.Containers {
		container := &spec.Containers[i]
		if !slices.ContainsFunc(container.VolumeMounts, func(m corev1.VolumeMount) bool {
			return m.Name == common.ConfigVolumeName && m.MountPath == common.ConfigVolumePath
		}) {
			continue
		}
		consumers++
		if !slices.ContainsFunc(container.VolumeMounts, func(m corev1.VolumeMount) bool {
			return m.MountPath == path.Join(common.ConfigVolumePath, "datadog.yaml") && m.SubPath != "" && m.ReadOnly
		}) {
			return fmt.Errorf("container %s requires an explicit read-only datadog.yaml file mount", container.Name)
		}
		for _, env := range container.Env {
			switch env.Name {
			case "DD_INSIDE_CI", "ECS_FARGATE", "ECS_MANAGED_INSTANCES", "DD_EKS_FARGATE", "MESOS_MASTER", "MESOS_SLAVE", "MARATHON_URL":
				if env.Value != "" || env.ValueFrom != nil {
					return fmt.Errorf("container %s sets %s, whose init script is outside this experiment", container.Name, env.Name)
				}
			}
		}
		for _, mount := range mounts {
			exists := false
			for _, current := range container.VolumeMounts {
				if apiequality.Semantic.DeepEqual(current, mount) {
					exists = true
					continue
				}
				if current.MountPath == common.ConfigVolumePath {
					continue
				}
				if current.MountPath == mount.MountPath || strings.HasPrefix(mount.MountPath, current.MountPath+"/") || strings.HasPrefix(current.MountPath, mount.MountPath+"/") {
					return fmt.Errorf("container %s has a conflicting mount at %s", container.Name, current.MountPath)
				}
			}
			if !exists {
				container.VolumeMounts = append(container.VolumeMounts, mount)
			}
		}
	}
	if consumers == 0 {
		return fmt.Errorf("no containers mount the shared configuration directory")
	}
	spec.InitContainers = slices.DeleteFunc(spec.InitContainers, func(c corev1.Container) bool { return c.Name == "init-config" })
	return nil
}

// configMapItems preserves explicit mappings. Without them, Kubernetes projects
// all ConfigMap keys as filenames, so discover the same keys for direct mounts.
func configMapItems(ctx context.Context, reader client.Reader, namespace string, volume corev1.Volume, custom *v2alpha1.MultiCustomConfig) ([]corev1.KeyToPath, error) {
	if len(volume.ConfigMap.Items) > 0 && (volume.ConfigMap.Optional == nil || !*volume.ConfigMap.Optional) {
		return volume.ConfigMap.Items, nil
	}
	keys := map[string]struct{}{}
	if custom != nil && custom.ConfigMap == nil && volume.ConfigMap.Name == strings.ToLower(string(v2alpha1.NodeAgentComponentName))+"-extra-"+volume.Name {
		// Inline ConfigMaps may not have been persisted yet. Their keys, including
		// an empty map, are already known from the desired configuration.
		for key := range custom.ConfigDataMap {
			keys[key] = struct{}{}
		}
	} else {
		cm := &corev1.ConfigMap{}
		if err := reader.Get(ctx, types.NamespacedName{Namespace: namespace, Name: volume.ConfigMap.Name}, cm); err != nil {
			if apierrors.IsNotFound(err) && volume.ConfigMap.Optional != nil && *volume.ConfigMap.Optional {
				return nil, nil
			}
			return nil, fmt.Errorf("read ConfigMap %s/%s for volume %s: %w", namespace, volume.ConfigMap.Name, volume.Name, err)
		}
		for key := range cm.Data {
			keys[key] = struct{}{}
		}
		for key := range cm.BinaryData {
			keys[key] = struct{}{}
		}
	}
	if len(volume.ConfigMap.Items) > 0 {
		// Optional projections omit absent keys. Do not create a subPath mount
		// for a file that Kubernetes will not project.
		var items []corev1.KeyToPath
		for _, item := range volume.ConfigMap.Items {
			if _, exists := keys[item.Key]; exists {
				items = append(items, item)
			}
		}
		return items, nil
	}
	ordered := make([]string, 0, len(keys))
	for key := range keys {
		ordered = append(ordered, key)
	}
	slices.Sort(ordered)
	items := make([]corev1.KeyToPath, 0, len(ordered))
	for _, key := range ordered {
		items = append(items, corev1.KeyToPath{Key: key, Path: key})
	}
	return items, nil
}

func safeConfigPath(file string) bool {
	if file == "" || path.IsAbs(file) || path.Clean(file) != file || strings.Contains(file, "\\") {
		return false
	}
	for part := range strings.SplitSeq(file, "/") {
		if part == "." || part == ".." {
			return false
		}
	}
	return true
}
