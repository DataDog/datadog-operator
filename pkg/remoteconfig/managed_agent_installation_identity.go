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
	managedAgentInstallationIDEnv         = "DD_EKS_INSTALLATION_ID"
	managedAgentInstallationEKSARNHashEnv = "DD_EKS_ARN_SHA256"
	managedAgentInstallationCapabilityTag = "managed_agent_installation:eks-addon-config-v1"
)

var managedAgentInstallationTargetHashPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

type ManagedAgentInstallationIdentity struct {
	InstallationID string
	TargetHash     string
}

func ManagedAgentInstallationIdentityFromEnvironment() (ManagedAgentInstallationIdentity, error) {
	installationID, installationIDConfigured := os.LookupEnv(managedAgentInstallationIDEnv)
	eksARNHash, eksARNHashConfigured := os.LookupEnv(managedAgentInstallationEKSARNHashEnv)
	if !installationIDConfigured && !eksARNHashConfigured {
		return ManagedAgentInstallationIdentity{}, nil
	}
	if !installationIDConfigured || !eksARNHashConfigured {
		return ManagedAgentInstallationIdentity{}, fmt.Errorf("EKS managed Agent installation identity requires both installation ID and ARN hash")
	}

	identity := ManagedAgentInstallationIdentity{
		InstallationID: installationID,
		TargetHash:     eksARNHash,
	}
	if err := identity.Validate(); err != nil {
		return ManagedAgentInstallationIdentity{}, err
	}
	return identity, nil
}

func (i ManagedAgentInstallationIdentity) Configured() bool {
	return i.InstallationID != "" || i.TargetHash != ""
}

func (i ManagedAgentInstallationIdentity) Validate() error {
	if !i.Configured() {
		return nil
	}
	if strings.TrimSpace(i.InstallationID) != i.InstallationID {
		return fmt.Errorf("EKS installation ID contains surrounding whitespace")
	}
	parsedInstallationID, err := uuid.Parse(i.InstallationID)
	if err != nil || parsedInstallationID == uuid.Nil || parsedInstallationID.String() != i.InstallationID {
		return fmt.Errorf("EKS installation ID must be a canonical non-zero UUID")
	}
	if !managedAgentInstallationTargetHashPattern.MatchString(i.TargetHash) {
		return fmt.Errorf("managed Agent installation target hash must be a lowercase SHA-256 digest")
	}
	return nil
}

func (i ManagedAgentInstallationIdentity) UpdaterTags() []string {
	if !i.Configured() || i.Validate() != nil {
		return nil
	}
	return []string{
		"eks_installation_id:" + i.InstallationID,
		"eks_arn_sha256:" + i.TargetHash,
		managedAgentInstallationCapabilityTag,
	}
}

func (i ManagedAgentInstallationIdentity) TargetID() string {
	digest, err := hex.DecodeString(i.TargetHash)
	if err != nil || len(digest) != 32 {
		return ""
	}
	return strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(digest))
}
