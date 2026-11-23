package eks

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

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

// OIDCProviderAPI is the subset of the IAM client used by ReuseOIDCProvider.
// Defined as an interface to allow mocking in tests.
type OIDCProviderAPI interface {
	ListOpenIDConnectProviders(ctx context.Context, params *iam.ListOpenIDConnectProvidersInput, optFns ...func(*iam.Options)) (*iam.ListOpenIDConnectProvidersOutput, error)
	GetOpenIDConnectProvider(ctx context.Context, params *iam.GetOpenIDConnectProviderInput, optFns ...func(*iam.Options)) (*iam.GetOpenIDConnectProviderOutput, error)
	AddClientIDToOpenIDConnectProvider(ctx context.Context, params *iam.AddClientIDToOpenIDConnectProviderInput, optFns ...func(*iam.Options)) (*iam.AddClientIDToOpenIDConnectProviderOutput, error)
}

// ReuseOIDCProvider returns the ARN of the IAM OIDC provider already
// registered for the given issuer URL, or an empty string if there is none.
//
// The AWS::IAM::OIDCProvider CloudFormation resource is not idempotent — it
// fails with EntityAlreadyExists when a provider with the same URL already
// exists. Clusters created with eksctl, or already set up with IRSA, commonly
// have a provider. Such a provider is reused as is, so that the stack only
// creates (and deletes) a provider when there is none.
func ReuseOIDCProvider(ctx context.Context, iamClient OIDCProviderAPI, issuerURL string) (string, error) {
	providerArn, provider, err := findOIDCProvider(ctx, iamClient, issuerURL)
	if err != nil || provider == nil {
		return "", err
	}

	// Ensure the STS audience is registered: the IRSA trust policy we
	// install conditions on `aud = sts.amazonaws.com`, which requires
	// that the provider itself lists that client ID. Providers
	// bootstrapped by other tools (eksctl, Terraform) may omit it.
	if !slices.Contains(provider.ClientIDList, stsAudience) {
		if _, err := iamClient.AddClientIDToOpenIDConnectProvider(ctx, &iam.AddClientIDToOpenIDConnectProviderInput{
			OpenIDConnectProviderArn: aws.String(providerArn),
			ClientID:                 aws.String(stsAudience),
		}); err != nil {
			return "", fmt.Errorf("failed to add %s client ID to OIDC provider %s: %w", stsAudience, providerArn, err)
		}
	}
	return providerArn, nil
}

// findOIDCProvider returns the ARN and the description of the IAM OIDC
// provider registered for the given issuer URL, or an empty ARN and a nil
// description if there is none.
//
// The provider is matched on its ARN, which embeds its URL
// (`arn:<partition>:iam::<account>:oidc-provider/<host>/<path>`), so that only
// the matching provider is described.
func findOIDCProvider(ctx context.Context, iamClient OIDCProviderAPI, issuerURL string) (string, *iam.GetOpenIDConnectProviderOutput, error) {
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
