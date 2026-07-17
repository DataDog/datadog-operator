// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package remoteconfig

import (
	"encoding/base32"
	"encoding/hex"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/google/uuid"
)

const (
	ManagedAgentInstallationProviderEKS ManagedAgentInstallationProvider = "eks"

	eksManagedAgentInstallationIDEnv         = "DD_EKS_INSTALLATION_ID"
	eksManagedAgentInstallationARNHashEnv    = "DD_EKS_ARN_SHA256"
	eksManagedAgentInstallationCapabilityTag = "managed_agent_installation:eks-addon-config-v1"
)

var eksManagedAgentInstallationTargetHashPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

type eksManagedAgentInstallationIdentity struct {
	installationID string
	targetHash     string
}

func NewEKSManagedAgentInstallationIdentity(installationID, targetHash string) ManagedAgentInstallationIdentity {
	return newManagedAgentInstallationIdentity(eksManagedAgentInstallationIdentity{
		installationID: installationID,
		targetHash:     targetHash,
	})
}

func (i eksManagedAgentInstallationIdentity) Provider() ManagedAgentInstallationProvider {
	return ManagedAgentInstallationProviderEKS
}

func (i eksManagedAgentInstallationIdentity) InstallationID() string {
	return i.installationID
}

func (i eksManagedAgentInstallationIdentity) TargetID() string {
	digest, err := hex.DecodeString(i.targetHash)
	if err != nil || len(digest) != 32 {
		return ""
	}
	return strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(digest))
}

func (i eksManagedAgentInstallationIdentity) Validate() error {
	if strings.TrimSpace(i.installationID) != i.installationID {
		return fmt.Errorf("EKS installation ID contains surrounding whitespace")
	}
	parsedInstallationID, err := uuid.Parse(i.installationID)
	if err != nil || parsedInstallationID == uuid.Nil || parsedInstallationID.String() != i.installationID {
		return fmt.Errorf("EKS installation ID must be a canonical non-zero UUID")
	}
	if !eksManagedAgentInstallationTargetHashPattern.MatchString(i.targetHash) {
		return fmt.Errorf("managed Agent installation target hash must be a lowercase SHA-256 digest")
	}
	return nil
}

func (i eksManagedAgentInstallationIdentity) UpdaterTags() []string {
	return []string{
		"eks_installation_id:" + i.installationID,
		"eks_arn_sha256:" + i.targetHash,
		eksManagedAgentInstallationCapabilityTag,
	}
}

func loadEKSManagedAgentInstallationIdentity() (ManagedAgentInstallationIdentity, error) {
	installationID, installationIDConfigured := os.LookupEnv(eksManagedAgentInstallationIDEnv)
	eksARNHash, eksARNHashConfigured := os.LookupEnv(eksManagedAgentInstallationARNHashEnv)
	if !installationIDConfigured && !eksARNHashConfigured {
		return ManagedAgentInstallationIdentity{}, nil
	}
	if !installationIDConfigured || !eksARNHashConfigured {
		return ManagedAgentInstallationIdentity{}, fmt.Errorf("EKS managed Agent installation identity requires both installation ID and ARN hash")
	}
	return NewEKSManagedAgentInstallationIdentity(installationID, eksARNHash), nil
}
