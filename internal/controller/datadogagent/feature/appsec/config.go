// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package appsec

import (
	"encoding/json"
	"fmt"
	"slices"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/validation"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
)

type Config struct {
	Enabled                   bool
	AutoDetect                *bool
	Proxies                   []string
	ProcessorAddress          string
	ProcessorPort             int
	ProcessorServiceName      string
	ProcessorServiceNamespace string
	// Sidecar injection mode fields
	Mode                           string
	SidecarImage                   string
	SidecarImageTag                string
	SidecarPort                    string
	SidecarHealthPort              string
	SidecarResourcesRequestsCPU    string
	SidecarResourcesRequestsMemory string
	SidecarResourcesLimitsCPU      string
	SidecarResourcesLimitsMemory   string
	SidecarBodyParsingSizeLimit    string
	// NginxModuleMountPath overrides the mount path for the nginx-datadog module inside the controller pod.
	// Maps to DD_ADMISSION_CONTROLLER_APPSEC_NGINX_MODULE_MOUNT_PATH on the cluster-agent.
	NginxModuleMountPath string
	// GatewayClasses lists the GKE GatewayClasses eligible for AppSec injection.
	// This field is CRD-only: there is no equivalent annotation.
	// Maps to DD_APPSEC_PROXY_GKE_GATEWAY_CLASSES on the cluster-agent.
	GatewayClasses []string
}

// configFromInjector builds a Config from the CRD injector configuration alone.
//
// The CRD and the deprecated `appsec.*` annotations are mutually exclusive sources:
// when spec.features.appsec.injector is present it defines the entire AppSec
// configuration and no annotation is consulted. Callers must therefore not seed this
// with annotation-derived values. See Feature.Configure for the source selection.
//
// Only the cpu and memory entries of the resource lists are honored; claims and any
// other resource name are ignored.
func configFromInjector(inj *v2alpha1.AppsecInjectorConfig) Config {
	var base Config

	if inj == nil {
		return base
	}

	if inj.Enabled != nil {
		base.Enabled = *inj.Enabled
	}

	if inj.AutoDetect != nil {
		// Copy the value instead of aliasing the pointer held by the DatadogAgent spec.
		autoDetect := *inj.AutoDetect
		base.AutoDetect = &autoDetect
	}

	if len(inj.Proxies) > 0 {
		// Clone to keep the feature config independent from the DatadogAgent spec.
		base.Proxies = slices.Clone(inj.Proxies)
	}

	if inj.Mode != nil {
		base.Mode = *inj.Mode
	}

	if inj.Processor != nil {
		if inj.Processor.Address != nil {
			base.ProcessorAddress = *inj.Processor.Address
		}
		if inj.Processor.Port != nil {
			base.ProcessorPort = int(*inj.Processor.Port)
		}
		if inj.Processor.Service != nil {
			if inj.Processor.Service.Name != nil {
				base.ProcessorServiceName = *inj.Processor.Service.Name
			}
			if inj.Processor.Service.Namespace != nil {
				base.ProcessorServiceNamespace = *inj.Processor.Service.Namespace
			}
		}
	}

	if inj.Sidecar != nil {
		if inj.Sidecar.Image != nil {
			base.SidecarImage = *inj.Sidecar.Image
		}
		if inj.Sidecar.ImageTag != nil {
			base.SidecarImageTag = *inj.Sidecar.ImageTag
		}
		if inj.Sidecar.Port != nil {
			base.SidecarPort = strconv.Itoa(int(*inj.Sidecar.Port))
		}
		if inj.Sidecar.HealthPort != nil {
			base.SidecarHealthPort = strconv.Itoa(int(*inj.Sidecar.HealthPort))
		}
		if inj.Sidecar.BodyParsingSizeLimit != nil {
			base.SidecarBodyParsingSizeLimit = strconv.FormatInt(*inj.Sidecar.BodyParsingSizeLimit, 10)
		}
		if inj.Sidecar.Resources != nil {
			if q, ok := inj.Sidecar.Resources.Requests[corev1.ResourceCPU]; ok {
				base.SidecarResourcesRequestsCPU = q.String()
			}
			if q, ok := inj.Sidecar.Resources.Requests[corev1.ResourceMemory]; ok {
				base.SidecarResourcesRequestsMemory = q.String()
			}
			if q, ok := inj.Sidecar.Resources.Limits[corev1.ResourceCPU]; ok {
				base.SidecarResourcesLimitsCPU = q.String()
			}
			if q, ok := inj.Sidecar.Resources.Limits[corev1.ResourceMemory]; ok {
				base.SidecarResourcesLimitsMemory = q.String()
			}
		}
	}

	if inj.Nginx != nil && inj.Nginx.ModuleMountPath != nil {
		base.NginxModuleMountPath = *inj.Nginx.ModuleMountPath
	}

	if inj.GKE != nil && len(inj.GKE.GatewayClasses) > 0 {
		base.GatewayClasses = slices.Clone(inj.GKE.GatewayClasses)
	}

	return base
}

// parseAnnotations builds a Config from the deprecated `appsec.*` annotations without
// validating it.
//
// This is only reached when spec.features.appsec.injector is absent: the CRD and the
// annotations are mutually exclusive sources, so there is no CRD value to defer to and
// every malformed annotation is an error.
//
// Errors are reported in source order, so a config with several malformed annotations
// always yields the same first error.
func parseAnnotations(annotations map[string]string) (config Config, err error) {
	// Read configuration from annotations

	if enabledStr, ok := annotations[AnnotationInjectorEnabled]; ok {
		if config.Enabled, err = strconv.ParseBool(enabledStr); err != nil {
			return config, fmt.Errorf("failed to parse annotation %q value: %w", AnnotationInjectorEnabled, err)
		}
	}

	if autoDetectStr, ok := annotations[AnnotationInjectorAutoDetect]; ok {
		autoDetect, parseErr := strconv.ParseBool(autoDetectStr)
		if parseErr != nil {
			return config, fmt.Errorf("failed to parse annotation %q value: %w", AnnotationInjectorAutoDetect, parseErr)
		}
		config.AutoDetect = &autoDetect
	}

	if proxiesStr, ok := annotations[AnnotationInjectorProxies]; ok && proxiesStr != "" {
		if parseErr := json.Unmarshal([]byte(proxiesStr), &config.Proxies); parseErr != nil {
			return config, fmt.Errorf("cannot parse annotation %q value: %w", AnnotationInjectorProxies, parseErr)
		}
	}

	config.ProcessorAddress = annotations[AnnotationInjectorProcessorAddress]
	config.ProcessorServiceName = annotations[AnnotationInjectorProcessorServiceName]
	config.ProcessorServiceNamespace = annotations[AnnotationInjectorProcessorServiceNamespace]

	if portStr, ok := annotations[AnnotationInjectorProcessorPort]; ok && portStr != "" {
		if config.ProcessorPort, err = strconv.Atoi(portStr); err != nil {
			return config, fmt.Errorf("cannot parse annotation %q value: %w", AnnotationInjectorProcessorPort, err)
		}
	}

	config.Mode = annotations[AnnotationInjectorMode]
	config.SidecarImage = annotations[AnnotationSidecarImage]
	config.SidecarImageTag = annotations[AnnotationSidecarImageTag]
	config.SidecarPort = annotations[AnnotationSidecarPort]
	config.SidecarHealthPort = annotations[AnnotationSidecarHealthPort]
	config.SidecarResourcesRequestsCPU = annotations[AnnotationSidecarResourcesRequestsCPU]
	config.SidecarResourcesRequestsMemory = annotations[AnnotationSidecarResourcesRequestsMemory]
	config.SidecarResourcesLimitsCPU = annotations[AnnotationSidecarResourcesLimitsCPU]
	config.SidecarResourcesLimitsMemory = annotations[AnnotationSidecarResourcesLimitsMemory]
	config.SidecarBodyParsingSizeLimit = annotations[AnnotationSidecarBodyParsingSizeLimit]
	config.NginxModuleMountPath = annotations[AnnotationNginxModuleMountPath]

	return config, nil
}

func (c Config) requiresNginxSupport() bool {
	if c.NginxModuleMountPath != "" {
		return true
	}
	return slices.Contains(c.Proxies, "ingress-nginx")
}

func (c Config) requiresGKESupport() bool {
	return slices.Contains(c.Proxies, "gke-gateway") || len(c.GatewayClasses) > 0
}

func (c Config) isEnabled() bool {
	if !c.Enabled {
		return false
	}

	if c.AutoDetect != nil && !*c.AutoDetect && len(c.Proxies) == 0 {
		return false
	}

	return true
}

// Validate checks that the Config has valid values for all fields.
// It returns an error if any validation fails.
func (c Config) Validate() error {
	if c.ProcessorPort < 0 || c.ProcessorPort > 65535 {
		return fmt.Errorf("processor port %d must be between 0 and 65535 (annotation: %s)",
			c.ProcessorPort, AnnotationInjectorProcessorPort)
	}

	for _, proxy := range c.Proxies {
		if !slices.Contains(AllowedProxyValues(), proxy) {
			return fmt.Errorf("invalid proxy value %q (allowed values: %v, annotation: %s)",
				proxy, AllowedProxyValues(), AnnotationInjectorProxies)
		}
	}

	if c.Mode != "" && c.Mode != "sidecar" && c.Mode != "external" {
		return fmt.Errorf("invalid mode %q (allowed values: sidecar, external, annotation: %s)",
			c.Mode, AnnotationInjectorMode)
	}

	// The gke-gateway proxy is only supported by the external processor, so it rejects
	// both sidecar mode and the empty default, which means sidecar. This message names
	// fields rather than an annotation because the config can also come from the CRD.
	if slices.Contains(c.Proxies, "gke-gateway") && c.Mode != "external" {
		return fmt.Errorf(`gke-gateway in proxies requires mode "external", got %q`, c.Mode)
	}

	// ProcessorServiceName is only required in external mode (not in sidecar mode, which is the default)
	if c.isEnabled() && c.Mode == "external" && c.ProcessorServiceName == "" {
		return fmt.Errorf("processor service name is required when AppSec is enabled in external mode (annotation: %s)",
			AnnotationInjectorProcessorServiceName)
	}

	if err := validatePort(c.SidecarPort, AnnotationSidecarPort); err != nil {
		return err
	}

	if err := validatePort(c.SidecarHealthPort, AnnotationSidecarHealthPort); err != nil {
		return err
	}

	if c.SidecarBodyParsingSizeLimit != "" {
		if _, err := strconv.ParseInt(c.SidecarBodyParsingSizeLimit, 10, 64); err != nil {
			return fmt.Errorf("cannot parse annotation %q value: %w", AnnotationSidecarBodyParsingSizeLimit, err)
		}
	}

	for val, annot := range map[string]string{
		c.SidecarResourcesRequestsCPU:    AnnotationSidecarResourcesRequestsCPU,
		c.SidecarResourcesRequestsMemory: AnnotationSidecarResourcesRequestsMemory,
		c.SidecarResourcesLimitsCPU:      AnnotationSidecarResourcesLimitsCPU,
		c.SidecarResourcesLimitsMemory:   AnnotationSidecarResourcesLimitsMemory,
	} {
		if val != "" {
			if _, err := resource.ParseQuantity(val); err != nil {
				return fmt.Errorf("invalid resource quantity %q for annotation %s: %w",
					val, annot, err)
			}
		}
	}

	return nil
}

// validatePort checks that a string port value, if non-empty, is a valid port number (1-65535).
func validatePort(portStr, annotation string) error {
	if portStr == "" {
		return nil
	}
	v, err := strconv.Atoi(portStr)
	if err != nil {
		return fmt.Errorf("cannot parse annotation %q value: %w", annotation, err)
	}
	if errs := validation.IsValidPortNum(v); len(errs) > 0 {
		return fmt.Errorf("invalid port for annotation %q: %s", annotation, errs[0])
	}
	return nil
}
