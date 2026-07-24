// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fleet

import "fmt"

type managedAgentInstallationIdentityLoader func() (ManagedAgentInstallationIdentity, error)

var managedAgentInstallationIdentityLoaders = []managedAgentInstallationIdentityLoader{
	loadEKSManagedAgentInstallationIdentity,
}

func ManagedAgentInstallationIdentityFromEnvironment() (ManagedAgentInstallationIdentity, error) {
	return managedAgentInstallationIdentityFromLoaders(managedAgentInstallationIdentityLoaders)
}

func managedAgentInstallationIdentityFromLoaders(loaders []managedAgentInstallationIdentityLoader) (ManagedAgentInstallationIdentity, error) {
	var configuredIdentity ManagedAgentInstallationIdentity
	for _, load := range loaders {
		identity, err := load()
		if err != nil {
			return ManagedAgentInstallationIdentity{}, err
		}
		if !identity.Configured() {
			continue
		}
		if configuredIdentity.Configured() {
			return ManagedAgentInstallationIdentity{}, fmt.Errorf("multiple managed Agent installation providers are configured")
		}
		if err := identity.Validate(); err != nil {
			return ManagedAgentInstallationIdentity{}, err
		}
		configuredIdentity = identity
	}
	return configuredIdentity, nil
}
