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
)

type Config struct {
	Enabled                   bool
	AutoDetect                *bool
	Proxies                   []string
	ProcessorAddress          string
	ProcessorPort             int
	ProcessorServiceName      string
	ProcessorServiceNamespace string
}

// FromAnnotations create a appsec.Config from an annotation map
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

func (c Config) validate() error {
	if c.ProcessorPort < 0 || c.ProcessorPort > 65535 {
		return fmt.Errorf("invalid configuration: port must be between 0 and 65535")
	}

	for _, proxy := range c.Proxies {
		if !slices.Contains(AllowedProxyValues, proxy) {
			return fmt.Errorf("invalid configuration: invalid proxy value %q (allowed values: %v)", proxy, AllowedProxyValues)
		}
	}

	if c.ProcessorServiceName == "" {
		return fmt.Errorf("invalid configuration: must specify a non-empty service name with a running service")
	}

	return nil
}
