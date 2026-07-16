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
	lifecycleInstallationIDEnv = "DD_EKS_INSTALLATION_ID"
	lifecycleEKSARNHashEnv     = "DD_EKS_ARN_SHA256"
	lifecycleCapabilityTag     = "operator_lifecycle:eks-addon-config-v1"
)

var lifecycleEKSARNHashPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

type LifecycleIdentity struct {
	InstallationID string
	EKSARNHash     string
}

func LifecycleIdentityFromEnvironment() (LifecycleIdentity, error) {
	installationID, installationIDConfigured := os.LookupEnv(lifecycleInstallationIDEnv)
	eksARNHash, eksARNHashConfigured := os.LookupEnv(lifecycleEKSARNHashEnv)
	if !installationIDConfigured && !eksARNHashConfigured {
		return LifecycleIdentity{}, nil
	}
	if !installationIDConfigured || !eksARNHashConfigured {
		return LifecycleIdentity{}, fmt.Errorf("EKS lifecycle identity requires both installation ID and ARN hash")
	}

	identity := LifecycleIdentity{
		InstallationID: installationID,
		EKSARNHash:     eksARNHash,
	}
	if err := identity.Validate(); err != nil {
		return LifecycleIdentity{}, err
	}
	return identity, nil
}

func (i LifecycleIdentity) Configured() bool {
	return i.InstallationID != "" || i.EKSARNHash != ""
}

func (i LifecycleIdentity) Validate() error {
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
	if !lifecycleEKSARNHashPattern.MatchString(i.EKSARNHash) {
		return fmt.Errorf("EKS ARN hash must be a lowercase SHA-256 digest")
	}
	return nil
}

func (i LifecycleIdentity) UpdaterTags() []string {
	if !i.Configured() || i.Validate() != nil {
		return nil
	}
	return []string{
		"eks_installation_id:" + i.InstallationID,
		"eks_arn_sha256:" + i.EKSARNHash,
		lifecycleCapabilityTag,
	}
}

func (i LifecycleIdentity) ARNLabelID() string {
	digest, err := hex.DecodeString(i.EKSARNHash)
	if err != nil || len(digest) != 32 {
		return ""
	}
	return strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(digest))
}
