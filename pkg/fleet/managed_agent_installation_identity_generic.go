// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fleet

import (
	"encoding/base32"
	"encoding/hex"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/google/uuid"
)

// Generic managed Agent installation identity, for installers that are not
// tied to a specific cloud (MCI on Nebius, CoreWeave and later clouds).
// Wire names are provider-neutral: env DD_MANAGED_INSTALLATION_ID and
// DD_RESOURCE_KEY_SHA256, updater tags managed_installation_id and
// resource_key_sha256, and the generic-v1 capability tag. The shape mirrors
// the EKS provider so Fleet Admin can select on either.
const (
	ManagedAgentInstallationProviderGeneric ManagedAgentInstallationProvider = "generic"

	genericManagedAgentInstallationIDEnv         = "DD_MANAGED_INSTALLATION_ID"
	genericManagedAgentInstallationKeyHashEnv    = "DD_RESOURCE_KEY_SHA256"
	genericManagedAgentInstallationCapabilityTag = "managed_agent_installation:generic-v1"
)

var genericManagedAgentInstallationTargetHashPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

type genericManagedAgentInstallationIdentity struct {
	installationID string
	targetHash     string
}

func NewGenericManagedAgentInstallationIdentity(installationID, resourceKeyHash string) ManagedAgentInstallationIdentity {
	return newManagedAgentInstallationIdentity(genericManagedAgentInstallationIdentity{
		installationID: installationID,
		targetHash:     resourceKeyHash,
	})
}

func (i genericManagedAgentInstallationIdentity) Provider() ManagedAgentInstallationProvider {
	return ManagedAgentInstallationProviderGeneric
}

func (i genericManagedAgentInstallationIdentity) InstallationID() string {
	return i.installationID
}

func (i genericManagedAgentInstallationIdentity) TargetID() string {
	digest, err := hex.DecodeString(i.targetHash)
	if err != nil || len(digest) != 32 {
		return ""
	}
	return strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(digest))
}

func (i genericManagedAgentInstallationIdentity) Validate() error {
	if strings.TrimSpace(i.installationID) != i.installationID {
		return fmt.Errorf("managed installation ID contains surrounding whitespace")
	}
	parsedInstallationID, err := uuid.Parse(i.installationID)
	if err != nil || parsedInstallationID == uuid.Nil || parsedInstallationID.String() != i.installationID {
		return fmt.Errorf("managed installation ID must be a canonical non-zero UUID")
	}
	if !genericManagedAgentInstallationTargetHashPattern.MatchString(i.targetHash) {
		return fmt.Errorf("managed Agent installation resource key hash must be a lowercase SHA-256 digest")
	}
	return nil
}

func (i genericManagedAgentInstallationIdentity) UpdaterTags() []string {
	return []string{
		"managed_installation_id:" + i.installationID,
		"resource_key_sha256:" + i.targetHash,
		genericManagedAgentInstallationCapabilityTag,
	}
}

func loadGenericManagedAgentInstallationIdentity() (ManagedAgentInstallationIdentity, error) {
	installationID, installationIDConfigured := os.LookupEnv(genericManagedAgentInstallationIDEnv)
	resourceKeyHash, resourceKeyHashConfigured := os.LookupEnv(genericManagedAgentInstallationKeyHashEnv)
	if !installationIDConfigured && !resourceKeyHashConfigured {
		return ManagedAgentInstallationIdentity{}, nil
	}
	if !installationIDConfigured || !resourceKeyHashConfigured {
		return ManagedAgentInstallationIdentity{}, fmt.Errorf("generic managed Agent installation identity requires both installation ID and resource key hash")
	}
	return NewGenericManagedAgentInstallationIdentity(installationID, resourceKeyHash), nil
}
