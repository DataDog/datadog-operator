package eks

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	ekstypes "github.com/aws/aws-sdk-go-v2/service/eks/types"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	iamtypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeIAM struct {
	providers map[string][]string // arn -> client IDs

	listErr           error
	getErr            error
	getCalls          int
	addClientIDInputs []*iam.AddClientIDToOpenIDConnectProviderInput
}

func (f *fakeIAM) ListOpenIDConnectProviders(_ context.Context, _ *iam.ListOpenIDConnectProvidersInput, _ ...func(*iam.Options)) (*iam.ListOpenIDConnectProvidersOutput, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	entries := make([]iamtypes.OpenIDConnectProviderListEntry, 0, len(f.providers))
	for arn := range f.providers {
		entries = append(entries, iamtypes.OpenIDConnectProviderListEntry{Arn: aws.String(arn)})
	}
	return &iam.ListOpenIDConnectProvidersOutput{OpenIDConnectProviderList: entries}, nil
}

func (f *fakeIAM) GetOpenIDConnectProvider(_ context.Context, params *iam.GetOpenIDConnectProviderInput, _ ...func(*iam.Options)) (*iam.GetOpenIDConnectProviderOutput, error) {
	f.getCalls++
	if f.getErr != nil {
		return nil, f.getErr
	}
	clientIDs, ok := f.providers[aws.ToString(params.OpenIDConnectProviderArn)]
	if !ok {
		return nil, &iamtypes.NoSuchEntityException{}
	}
	return &iam.GetOpenIDConnectProviderOutput{ClientIDList: clientIDs}, nil
}

func (f *fakeIAM) AddClientIDToOpenIDConnectProvider(_ context.Context, params *iam.AddClientIDToOpenIDConnectProviderInput, _ ...func(*iam.Options)) (*iam.AddClientIDToOpenIDConnectProviderOutput, error) {
	f.addClientIDInputs = append(f.addClientIDInputs, params)
	return &iam.AddClientIDToOpenIDConnectProviderOutput{}, nil
}

func TestReuseOIDCProvider(t *testing.T) {
	const (
		issuerURL = "https://oidc.eks.eu-west-3.amazonaws.com/id/ABCDEF"
		issuerArn = "arn:aws:iam::123456789012:oidc-provider/oidc.eks.eu-west-3.amazonaws.com/id/ABCDEF"
		otherArn  = "arn:aws:iam::123456789012:oidc-provider/oidc.eks.eu-west-3.amazonaws.com/id/OTHER"
	)
	sts := []string{"sts.amazonaws.com"}

	for _, tc := range []struct {
		name      string
		existing  map[string][]string
		listErr   error
		getErr    error
		issuerURL string
		// expectArn is the returned ARN, empty when no provider is registered
		// for the issuer and the stack must create one.
		expectArn       string
		expectAddClient bool // true when the existing provider must be patched to add the STS audience
		expectGetCalls  int
		expectError     string
	}{
		{
			name:           "provider already exists with STS audience",
			existing:       map[string][]string{otherArn: nil, issuerArn: sts},
			issuerURL:      issuerURL,
			expectArn:      issuerArn,
			expectGetCalls: 1, // only the matching provider is described
		},
		{
			name:            "provider exists without STS audience is patched in place",
			existing:        map[string][]string{issuerArn: {"other.audience"}},
			issuerURL:       issuerURL,
			expectArn:       issuerArn,
			expectAddClient: true,
			expectGetCalls:  1,
		},
		{
			name:           "host match is case-insensitive (RFC 3986)",
			existing:       map[string][]string{issuerArn: sts},
			issuerURL:      "https://OIDC.EKS.EU-WEST-3.AMAZONAWS.COM/id/ABCDEF",
			expectArn:      issuerArn,
			expectGetCalls: 1,
		},
		{
			name:      "path match is case-sensitive (RFC 3986)",
			existing:  map[string][]string{"arn:aws:iam::123456789012:oidc-provider/oidc.eks.eu-west-3.amazonaws.com/id/abcdef": sts},
			issuerURL: issuerURL,
		},
		{
			name:      "no provider when list is an empty map",
			existing:  map[string][]string{},
			issuerURL: issuerURL,
		},
		{
			name:      "no provider when list is nil",
			existing:  nil,
			issuerURL: issuerURL,
		},
		{
			name:           "provider deleted between list and get",
			existing:       map[string][]string{issuerArn: nil},
			getErr:         &iamtypes.NoSuchEntityException{},
			issuerURL:      issuerURL,
			expectGetCalls: 1,
		},
		{
			name:           "get error propagates",
			existing:       map[string][]string{issuerArn: nil},
			getErr:         errors.New("access denied"),
			issuerURL:      issuerURL,
			expectGetCalls: 1,
			expectError:    "failed to get OIDC provider",
		},
		{
			name:        "list error propagates",
			listErr:     errors.New("api throttled"),
			issuerURL:   issuerURL,
			expectError: "failed to list OIDC providers",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeIAM{
				providers: tc.existing,
				listErr:   tc.listErr,
				getErr:    tc.getErr,
			}

			arn, err := ReuseOIDCProvider(t.Context(), f, tc.issuerURL)

			assert.Equal(t, tc.expectGetCalls, f.getCalls)
			if tc.expectError != "" {
				require.ErrorContains(t, err, tc.expectError)
				return
			}
			require.NoError(t, err)

			assert.Equal(t, tc.expectArn, arn)
			if tc.expectAddClient {
				require.Len(t, f.addClientIDInputs, 1, "AddClientIDToOpenIDConnectProvider should have been called")
				assert.Equal(t, "sts.amazonaws.com", aws.ToString(f.addClientIDInputs[0].ClientID))
			} else {
				assert.Empty(t, f.addClientIDInputs, "AddClientIDToOpenIDConnectProvider must not be called when STS audience is already registered")
			}
		})
	}
}

func TestGetClusterOIDCIssuerURL(t *testing.T) {
	for _, tc := range []struct {
		name        string
		cluster     *ekstypes.Cluster
		expected    string
		expectError bool
	}{
		{
			name: "valid cluster with OIDC issuer",
			cluster: &ekstypes.Cluster{
				Identity: &ekstypes.Identity{
					Oidc: &ekstypes.OIDC{
						Issuer: aws.String("https://oidc.eks.eu-west-3.amazonaws.com/id/ABCDEF"),
					},
				},
			},
			expected: "https://oidc.eks.eu-west-3.amazonaws.com/id/ABCDEF",
		},
		{
			name:        "nil cluster",
			cluster:     nil,
			expectError: true,
		},
		{
			name:        "cluster without identity",
			cluster:     &ekstypes.Cluster{},
			expectError: true,
		},
		{
			name: "cluster with empty issuer URL",
			cluster: &ekstypes.Cluster{
				Identity: &ekstypes.Identity{Oidc: &ekstypes.OIDC{Issuer: aws.String("")}},
			},
			expectError: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := GetClusterOIDCIssuerURL(tc.cluster)
			if tc.expectError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.expected, got)
		})
	}
}
