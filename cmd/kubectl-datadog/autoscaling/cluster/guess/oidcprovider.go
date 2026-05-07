package guess

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	ekstypes "github.com/aws/aws-sdk-go-v2/service/eks/types"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	iamtypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
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

// OIDCProviderAPI is the subset of the IAM client used by EnsureOIDCProvider.
// Defined as an interface to allow mocking in tests.
type OIDCProviderAPI interface {
	ListOpenIDConnectProviders(ctx context.Context, params *iam.ListOpenIDConnectProvidersInput, optFns ...func(*iam.Options)) (*iam.ListOpenIDConnectProvidersOutput, error)
	GetOpenIDConnectProvider(ctx context.Context, params *iam.GetOpenIDConnectProviderInput, optFns ...func(*iam.Options)) (*iam.GetOpenIDConnectProviderOutput, error)
	CreateOpenIDConnectProvider(ctx context.Context, params *iam.CreateOpenIDConnectProviderInput, optFns ...func(*iam.Options)) (*iam.CreateOpenIDConnectProviderOutput, error)
	AddClientIDToOpenIDConnectProvider(ctx context.Context, params *iam.AddClientIDToOpenIDConnectProviderInput, optFns ...func(*iam.Options)) (*iam.AddClientIDToOpenIDConnectProviderOutput, error)
}

// EnsureOIDCProvider returns the ARN of an IAM OIDC provider for the given
// issuer URL, creating one if it does not already exist.
//
// The AWS::IAM::OIDCProvider CloudFormation resource is not idempotent — it
// fails with EntityAlreadyExists when a provider with the same URL already
// exists. Clusters created with eksctl, or already set up with IRSA, commonly
// have a provider. Bootstrapping in Go allows check-then-create.
//
// The provider is never deleted on uninstall because it may be shared with
// other workloads in the cluster.
func EnsureOIDCProvider(ctx context.Context, iamClient OIDCProviderAPI, issuerURL string) (string, error) {
	targetURL := normalizeOIDCURL(issuerURL)

	listOut, err := iamClient.ListOpenIDConnectProviders(ctx, &iam.ListOpenIDConnectProvidersInput{})
	if err != nil {
		return "", fmt.Errorf("failed to list OIDC providers: %w", err)
	}

	for _, provider := range listOut.OpenIDConnectProviderList {
		providerArn := aws.ToString(provider.Arn)
		if providerArn == "" {
			continue
		}
		getOut, getErr := iamClient.GetOpenIDConnectProvider(ctx, &iam.GetOpenIDConnectProviderInput{
			OpenIDConnectProviderArn: provider.Arn,
		})
		if getErr != nil {
			return "", fmt.Errorf("failed to get OIDC provider %s: %w", providerArn, getErr)
		}
		if normalizeOIDCURL(aws.ToString(getOut.Url)) == targetURL {
			// Ensure the STS audience is registered: the IRSA trust policy we
			// install conditions on `aud = sts.amazonaws.com`, which requires
			// that the provider itself lists that client ID. Providers
			// bootstrapped by other tools (eksctl, Terraform) may omit it.
			if !slices.Contains(getOut.ClientIDList, stsAudience) {
				if _, addErr := iamClient.AddClientIDToOpenIDConnectProvider(ctx, &iam.AddClientIDToOpenIDConnectProviderInput{
					OpenIDConnectProviderArn: provider.Arn,
					ClientID:                 aws.String(stsAudience),
				}); addErr != nil {
					return "", fmt.Errorf("failed to add %s client ID to OIDC provider %s: %w", stsAudience, providerArn, addErr)
				}
			}
			return providerArn, nil
		}
	}

	createOut, err := iamClient.CreateOpenIDConnectProvider(ctx, &iam.CreateOpenIDConnectProviderInput{
		Url:          aws.String(issuerURL),
		ClientIDList: []string{stsAudience},
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
		var notFound *iamtypes.NoSuchEntityException
		if !errors.As(err, &notFound) {
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
func normalizeOIDCURL(url string) string {
	url = strings.TrimPrefix(url, "https://")
	slash := strings.Index(url, "/")
	if slash < 0 {
		return strings.ToLower(url)
	}
	return strings.ToLower(url[:slash]) + url[slash:]
}
