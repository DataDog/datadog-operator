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

	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/validation"
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
}

// FromAnnotations creates an appsec.Config from an annotation map and validates it.
// It parses annotations with the "agent.datadoghq.com/appsec.injector.*" prefix
// to configure the AppSec proxy injection feature.
//
// Returns an error if:
//   - Boolean values cannot be parsed (enabled, autoDetect)
//   - Proxies JSON is malformed
//   - Port is not a valid integer
//   - Configuration validation fails (invalid port range, invalid proxy values, missing required fields)
func FromAnnotations(annotations map[string]string) (config Config, err error) {
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

	// Validate the configuration before returning
	if err = config.Validate(); err != nil {
		return config, fmt.Errorf("invalid configuration: %w", err)
	}

	return config, nil
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
