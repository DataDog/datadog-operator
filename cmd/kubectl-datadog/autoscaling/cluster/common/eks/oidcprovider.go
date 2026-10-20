package eks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	ekstypes "github.com/aws/aws-sdk-go-v2/service/eks/types"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	iamtypes "github.com/aws/aws-sdk-go-v2/service/iam/types"

	commonaws "github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/cluster/common/aws"
)

const stsAudience = "sts.amazonaws.com"

// GetClusterOIDCIssuerURL returns the cluster's OIDC issuer URL, with its
// `https://` prefix (as returned by the EKS API). Returns an error if the
// cluster has no OIDC identity configured.
func GetClusterOIDCIssuerURL(cluster *ekstypes.Cluster) (string, error) {
	if cluster == nil || cluster.Identity == nil || cluster.Identity.Oidc == nil {
		return "", fmt.Errorf("cluster has no OIDC identity")
	}
	issuer := aws.ToString(cluster.Identity.Oidc.Issuer)
	if issuer == "" {
		return "", fmt.Errorf("cluster OIDC issuer URL is empty")
	}
	return issuer, nil
}

// OIDCProviderAPI is the subset of the IAM client used by EnsureOIDCProvider
// and DeleteOwnedOIDCProvider. Defined as an interface to allow mocking in
// tests.
type OIDCProviderAPI interface {
	ListOpenIDConnectProviders(ctx context.Context, params *iam.ListOpenIDConnectProvidersInput, optFns ...func(*iam.Options)) (*iam.ListOpenIDConnectProvidersOutput, error)
	GetOpenIDConnectProvider(ctx context.Context, params *iam.GetOpenIDConnectProviderInput, optFns ...func(*iam.Options)) (*iam.GetOpenIDConnectProviderOutput, error)
	CreateOpenIDConnectProvider(ctx context.Context, params *iam.CreateOpenIDConnectProviderInput, optFns ...func(*iam.Options)) (*iam.CreateOpenIDConnectProviderOutput, error)
	AddClientIDToOpenIDConnectProvider(ctx context.Context, params *iam.AddClientIDToOpenIDConnectProviderInput, optFns ...func(*iam.Options)) (*iam.AddClientIDToOpenIDConnectProviderOutput, error)
	iam.ListRolesAPIClient
	DeleteOpenIDConnectProvider(ctx context.Context, params *iam.DeleteOpenIDConnectProviderInput, optFns ...func(*iam.Options)) (*iam.DeleteOpenIDConnectProviderOutput, error)
}

// EnsureOIDCProvider returns the ARN of an IAM OIDC provider for the given
// issuer URL, creating one if it does not already exist.
//
// The AWS::IAM::OIDCProvider CloudFormation resource is not idempotent — it
// fails with EntityAlreadyExists when a provider with the same URL already
// exists. Clusters created with eksctl, or already set up with IRSA, commonly
// have a provider. Bootstrapping in Go allows check-then-create.
//
// A provider created here is tagged as managed by kubectl-datadog so that
// DeleteOwnedOIDCProvider can remove it on uninstall. A pre-existing provider
// is reused as is and never tagged, so that uninstall leaves it alone: it may
// be shared with other workloads in the cluster.
func EnsureOIDCProvider(ctx context.Context, iamClient OIDCProviderAPI, issuerURL string) (string, error) {
	providerArn, provider, err := FindOIDCProvider(ctx, iamClient, issuerURL)
	if err != nil {
		return "", err
	}

	if provider != nil {
		// Ensure the STS audience is registered: the IRSA trust policy we
		// install conditions on `aud = sts.amazonaws.com`, which requires
		// that the provider itself lists that client ID. Providers
		// bootstrapped by other tools (eksctl, Terraform) may omit it.
		if !slices.Contains(provider.ClientIDList, stsAudience) {
			if _, addErr := iamClient.AddClientIDToOpenIDConnectProvider(ctx, &iam.AddClientIDToOpenIDConnectProviderInput{
				OpenIDConnectProviderArn: aws.String(providerArn),
				ClientID:                 aws.String(stsAudience),
			}); addErr != nil {
				return "", fmt.Errorf("failed to add %s client ID to OIDC provider %s: %w", stsAudience, providerArn, addErr)
			}
		}
		return providerArn, nil
	}

	createOut, err := iamClient.CreateOpenIDConnectProvider(ctx, &iam.CreateOpenIDConnectProviderInput{
		Url:          aws.String(issuerURL),
		ClientIDList: []string{stsAudience},
		Tags: []iamtypes.Tag{{
			Key:   aws.String(commonaws.ManagedByTag),
			Value: aws.String(commonaws.ManagedByTagValue),
		}},
	})
	if err != nil {
		return "", fmt.Errorf("failed to create OIDC provider for %s: %w", issuerURL, err)
	}
	arn := aws.ToString(createOut.OpenIDConnectProviderArn)

	// IAM is eventually consistent: the CloudFormation stack created right
	// after references this provider as the federated principal of the
	// Karpenter IRSA role. Poll GetOpenIDConnectProvider until the provider
	// is readable back, so the stack creation doesn't race with propagation.
	if err := waitForOIDCProviderReadable(ctx, iamClient, arn); err != nil {
		return "", err
	}
	return arn, nil
}

// DeleteOwnedOIDCProvider deletes the IAM OIDC provider registered for the
// given issuer URL if, and only if, it was created by EnsureOIDCProvider and
// no IAM role other than ownRoleName trusts it. It returns the ARN of the
// deleted provider, or an empty ARN and the reason why nothing was deleted.
//
// The IAM role check protects workloads that started to rely on the provider
// after kubectl-datadog created it. ownRoleName is the Karpenter IRSA role
// created by kubectl-datadog: it is ignored because IAM ListRoles is
// eventually consistent and may still return it right after its deletion.
func DeleteOwnedOIDCProvider(ctx context.Context, iamClient OIDCProviderAPI, issuerURL string, ownRoleName string) (deletedArn string, skipReason string, err error) {
	providerArn, provider, err := FindOIDCProvider(ctx, iamClient, issuerURL)
	if err != nil {
		return "", "", err
	}
	if provider == nil {
		return "", "no OIDC provider is registered for " + issuerURL, nil
	}

	if !isManagedByKubectlDatadog(provider.Tags) {
		return "", "OIDC provider " + providerArn + " was not created by kubectl-datadog", nil
	}

	trustingRoles, err := listRolesTrusting(ctx, iamClient, providerArn, ownRoleName)
	if err != nil {
		return "", "", err
	}
	if len(trustingRoles) > 0 {
		return "", "OIDC provider " + providerArn + " is still trusted by IAM role(s) " + strings.Join(trustingRoles, ", "), nil
	}

	if _, err := iamClient.DeleteOpenIDConnectProvider(ctx, &iam.DeleteOpenIDConnectProviderInput{
		OpenIDConnectProviderArn: aws.String(providerArn),
	}); err != nil {
		if _, ok := errors.AsType[*iamtypes.NoSuchEntityException](err); ok {
			return "", "OIDC provider " + providerArn + " is already deleted", nil
		}
		return "", "", fmt.Errorf("failed to delete OIDC provider %s: %w", providerArn, err)
	}
	return providerArn, "", nil
}

// FindOIDCProvider returns the ARN and the description of the IAM OIDC
// provider registered for the given issuer URL, or an empty ARN and a nil
// description if there is none.
//
// The provider is matched on its ARN, which embeds its URL
// (`arn:<partition>:iam::<account>:oidc-provider/<host>/<path>`), so that only
// the matching provider is described.
func FindOIDCProvider(ctx context.Context, iamClient OIDCProviderAPI, issuerURL string) (string, *iam.GetOpenIDConnectProviderOutput, error) {
	targetURL := normalizeOIDCURL(issuerURL)

	listOut, err := iamClient.ListOpenIDConnectProviders(ctx, &iam.ListOpenIDConnectProvidersInput{})
	if err != nil {
		return "", nil, fmt.Errorf("failed to list OIDC providers: %w", err)
	}

	for _, provider := range listOut.OpenIDConnectProviderList {
		providerArn := aws.ToString(provider.Arn)
		_, providerURL, found := strings.Cut(providerArn, ":oidc-provider/")
		if !found || normalizeOIDCURL(providerURL) != targetURL {
			continue
		}
		getOut, err := iamClient.GetOpenIDConnectProvider(ctx, &iam.GetOpenIDConnectProviderInput{
			OpenIDConnectProviderArn: provider.Arn,
		})
		if err != nil {
			if _, ok := errors.AsType[*iamtypes.NoSuchEntityException](err); ok {
				// Deleted since it was listed.
				return "", nil, nil
			}
			return "", nil, fmt.Errorf("failed to get OIDC provider %s: %w", providerArn, err)
		}
		return providerArn, getOut, nil
	}
	return "", nil, nil
}

func isManagedByKubectlDatadog(tags []iamtypes.Tag) bool {
	return slices.ContainsFunc(tags, func(t iamtypes.Tag) bool {
		return aws.ToString(t.Key) == commonaws.ManagedByTag && aws.ToString(t.Value) == commonaws.ManagedByTagValue
	})
}

// listRolesTrusting returns the names of the IAM roles, other than
// ignoredRoleName, whose trust policy references the given OIDC provider ARN
// or cannot be decoded.
func listRolesTrusting(ctx context.Context, iamClient OIDCProviderAPI, providerArn string, ignoredRoleName string) ([]string, error) {
	var roles []string
	paginator := iam.NewListRolesPaginator(iamClient, &iam.ListRolesInput{MaxItems: aws.Int32(1000)})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list IAM roles: %w", err)
		}
		for _, role := range page.Roles {
			name := aws.ToString(role.RoleName)
			if name == ignoredRoleName {
				continue
			}
			trusts, err := trustPolicyReferences(aws.ToString(role.AssumeRolePolicyDocument), providerArn)
			// A trust policy that cannot be decoded may trust the provider.
			if err != nil || trusts {
				roles = append(roles, name)
			}
		}
	}
	return roles, nil
}

// trustPolicyReferences reports whether any value of the given trust policy
// document, as returned URL-encoded by IAM, is the given ARN.
//
// The document is decoded as a form value, like java.net.URLDecoder which
// the IAM documentation suggests: a `+` standing for a space would otherwise
// be left as is and make the JSON invalid.
func trustPolicyReferences(encoded string, arn string) (bool, error) {
	raw, err := url.QueryUnescape(encoded)
	if err != nil {
		return false, err
	}
	var doc any
	if err = json.Unmarshal([]byte(raw), &doc); err != nil {
		return false, err
	}
	return jsonContainsString(doc, arn), nil
}

// jsonContainsString reports whether s is one of the string values of the
// decoded JSON value v.
func jsonContainsString(v any, s string) bool {
	switch v := v.(type) {
	case string:
		return v == s
	case []any:
		return slices.ContainsFunc(v, func(e any) bool { return jsonContainsString(e, s) })
	case map[string]any:
		for _, e := range v {
			if jsonContainsString(e, s) {
				return true
			}
		}
	}
	return false
}

// waitForOIDCProviderReadable polls GetOpenIDConnectProvider until the given
// provider ARN is readable, with a short bounded retry budget. IAM
// CreateOpenIDConnectProvider is asynchronous from a read-consistency
// standpoint; in practice the provider is readable within seconds.
func waitForOIDCProviderReadable(ctx context.Context, iamClient OIDCProviderAPI, arn string) error {
	const (
		maxAttempts = 20
		interval    = 500 * time.Millisecond
	)
	for range maxAttempts {
		_, err := iamClient.GetOpenIDConnectProvider(ctx, &iam.GetOpenIDConnectProviderInput{
			OpenIDConnectProviderArn: aws.String(arn),
		})
		if err == nil {
			return nil
		}
		if _, ok := errors.AsType[*iamtypes.NoSuchEntityException](err); !ok {
			return fmt.Errorf("failed to read back OIDC provider %s: %w", arn, err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(interval):
		}
	}
	return fmt.Errorf("OIDC provider %s not readable after %v", arn, maxAttempts*interval)
}

// normalizeOIDCURL strips the `https://` scheme and lowercases the host so
// that the URL can be compared reliably with what IAM stores (no scheme).
// The path segment is left case-sensitive because per RFC 3986 only the
// scheme and host are case-insensitive — an OIDC provider URL with an
// uppercase path ID must NOT collide with a different URL whose path only
// differs by case.
func normalizeOIDCURL(rawURL string) string {
	rawURL = strings.TrimPrefix(rawURL, "https://")
	slash := strings.Index(rawURL, "/")
	if slash < 0 {
		return strings.ToLower(rawURL)
	}
	return strings.ToLower(rawURL[:slash]) + rawURL[slash:]
}
