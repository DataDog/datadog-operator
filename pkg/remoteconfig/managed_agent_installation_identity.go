// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package remoteconfig

import "fmt"

type ManagedAgentInstallationProvider string

type managedAgentInstallationProviderIdentity interface {
	Provider() ManagedAgentInstallationProvider
	InstallationID() string
	TargetID() string
	Validate() error
	UpdaterTags() []string
}

type ManagedAgentInstallationIdentity struct {
	identity managedAgentInstallationProviderIdentity
}

func newManagedAgentInstallationIdentity(identity managedAgentInstallationProviderIdentity) ManagedAgentInstallationIdentity {
	return ManagedAgentInstallationIdentity{identity: identity}
}

func (i ManagedAgentInstallationIdentity) Configured() bool {
	return i.identity != nil
}

func (i ManagedAgentInstallationIdentity) Provider() ManagedAgentInstallationProvider {
	if !i.Configured() {
		return ""
	}
	return i.identity.Provider()
}

func (i ManagedAgentInstallationIdentity) InstallationID() string {
	if !i.Configured() {
		return ""
	}
	return i.identity.InstallationID()
}

func (i ManagedAgentInstallationIdentity) Validate() error {
	if !i.Configured() {
		return nil
	}
	return i.identity.Validate()
}

func (i ManagedAgentInstallationIdentity) UpdaterTags() ([]string, error) {
	if !i.Configured() {
		return nil, nil
	}
	if err := i.Validate(); err != nil {
		return nil, fmt.Errorf("validate %q managed Agent installation identity: %w", i.Provider(), err)
	}
	return i.identity.UpdaterTags(), nil
}

func (i ManagedAgentInstallationIdentity) TargetID() string {
	if !i.Configured() {
		return ""
	}
	return i.identity.TargetID()
}
