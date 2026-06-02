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

type fakeProvider struct {
	url          string
	clientIDList []string
}

type fakeIAM struct {
	providers map[string]*fakeProvider // arn -> provider

	createCalls       int
	createInput       *iam.CreateOpenIDConnectProviderInput // captured on Create
	createErr         error
	listErr           error
	getErr            error
	addClientIDCalls  int
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
	if f.getErr != nil {
		return nil, f.getErr
	}
	p, ok := f.providers[aws.ToString(params.OpenIDConnectProviderArn)]
	if !ok {
		return nil, errors.New("not found")
	}
	return &iam.GetOpenIDConnectProviderOutput{
		Url:          aws.String(p.url),
		ClientIDList: p.clientIDList,
	}, nil
}

func (f *fakeIAM) CreateOpenIDConnectProvider(_ context.Context, params *iam.CreateOpenIDConnectProviderInput, _ ...func(*iam.Options)) (*iam.CreateOpenIDConnectProviderOutput, error) {
	f.createCalls++
	f.createInput = params
	if f.createErr != nil {
		return nil, f.createErr
	}
	normalized := normalizeOIDCURL(aws.ToString(params.Url))
	arn := "arn:aws:iam::123456789012:oidc-provider/" + normalized
	if f.providers == nil {
		f.providers = map[string]*fakeProvider{}
	}
	f.providers[arn] = &fakeProvider{url: normalized, clientIDList: params.ClientIDList}
	return &iam.CreateOpenIDConnectProviderOutput{OpenIDConnectProviderArn: aws.String(arn)}, nil
}

func (f *fakeIAM) AddClientIDToOpenIDConnectProvider(_ context.Context, params *iam.AddClientIDToOpenIDConnectProviderInput, _ ...func(*iam.Options)) (*iam.AddClientIDToOpenIDConnectProviderOutput, error) {
	f.addClientIDCalls++
	f.addClientIDInputs = append(f.addClientIDInputs, params)
	if p, ok := f.providers[aws.ToString(params.OpenIDConnectProviderArn)]; ok {
		p.clientIDList = append(p.clientIDList, aws.ToString(params.ClientID))
	}
	return &iam.AddClientIDToOpenIDConnectProviderOutput{}, nil
}

func TestEnsureOIDCProvider(t *testing.T) {
	const (
		issuerURL   = "https://oidc.eks.eu-west-3.amazonaws.com/id/ABCDEF"
		issuerStore = "oidc.eks.eu-west-3.amazonaws.com/id/ABCDEF"
		issuerArn   = "arn:aws:iam::123456789012:oidc-provider/" + issuerStore
	)

	for _, tc := range []struct {
		name          string
		existing      map[string]*fakeProvider
		listErr       error
		createErr     error
		issuerURL     string
		expectError   bool
		errorContains string
		// When Create is NOT expected: expectArn is checked against the returned ARN.
		// When Create IS expected: expectCreateURL holds the URL Create must receive.
		expectArn       string
		expectCreateURL string
		expectAddClient bool // true when the existing provider must be patched to add the STS audience
	}{
		{
			name:      "provider already exists with STS audience",
			existing:  map[string]*fakeProvider{issuerArn: {url: issuerStore, clientIDList: []string{"sts.amazonaws.com"}}},
			issuerURL: issuerURL,
			expectArn: issuerArn,
		},
		{
			name:            "provider exists without STS audience is patched in place",
			existing:        map[string]*fakeProvider{issuerArn: {url: issuerStore, clientIDList: []string{"other.audience"}}},
			issuerURL:       issuerURL,
			expectArn:       issuerArn,
			expectAddClient: true,
		},
		{
			name:      "host match is case-insensitive (RFC 3986)",
			existing:  map[string]*fakeProvider{issuerArn: {url: issuerStore, clientIDList: []string{"sts.amazonaws.com"}}},
			issuerURL: "https://OIDC.EKS.EU-WEST-3.AMAZONAWS.COM/id/ABCDEF",
			expectArn: issuerArn,
		},
		{
			name: "path match is case-sensitive (RFC 3986)",
			existing: map[string]*fakeProvider{
				"arn:aws:iam::123456789012:oidc-provider/oidc.eks.eu-west-3.amazonaws.com/id/abcdef": {
					url:          "oidc.eks.eu-west-3.amazonaws.com/id/abcdef",
					clientIDList: []string{"sts.amazonaws.com"},
				},
			},
			issuerURL:       issuerURL,
			expectCreateURL: issuerURL,
		},
		{
			name:            "creates when list is an empty map",
			existing:        map[string]*fakeProvider{},
			issuerURL:       "https://oidc.eks.eu-west-3.amazonaws.com/id/NEW",
			expectCreateURL: "https://oidc.eks.eu-west-3.amazonaws.com/id/NEW",
		},
		{
			name:            "creates when list is nil",
			existing:        nil,
			issuerURL:       "https://oidc.eks.eu-west-3.amazonaws.com/id/XYZ",
			expectCreateURL: "https://oidc.eks.eu-west-3.amazonaws.com/id/XYZ",
		},
		{
			name:          "list error propagates",
			listErr:       errors.New("api throttled"),
			issuerURL:     issuerURL,
			expectError:   true,
			errorContains: "failed to list OIDC providers",
		},
		{
			name:          "create error propagates",
			existing:      map[string]*fakeProvider{},
			createErr:     errors.New("quota exceeded"),
			issuerURL:     issuerURL,
			expectError:   true,
			errorContains: "failed to create OIDC provider",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeIAM{
				providers: tc.existing,
				listErr:   tc.listErr,
				createErr: tc.createErr,
			}

			arn, err := EnsureOIDCProvider(t.Context(), f, tc.issuerURL)

			if tc.expectError {
				require.Error(t, err)
				if tc.errorContains != "" {
					assert.Contains(t, err.Error(), tc.errorContains)
				}
				return
			}
			require.NoError(t, err)

			if tc.expectCreateURL == "" {
				assert.Equal(t, 0, f.createCalls, "Create must not be called when an existing provider matches")
				assert.Equal(t, tc.expectArn, arn)
				if tc.expectAddClient {
					assert.Equal(t, 1, f.addClientIDCalls, "AddClientIDToOpenIDConnectProvider should have been called")
					require.Len(t, f.addClientIDInputs, 1)
					assert.Equal(t, "sts.amazonaws.com", aws.ToString(f.addClientIDInputs[0].ClientID))
				} else {
					assert.Equal(t, 0, f.addClientIDCalls, "AddClientIDToOpenIDConnectProvider must not be called when STS audience is already registered")
				}
				return
			}

			assert.Equal(t, 1, f.createCalls)
			require.NotNil(t, f.createInput)
			assert.Equal(t, tc.expectCreateURL, aws.ToString(f.createInput.Url))
			assert.Equal(t, []string{"sts.amazonaws.com"}, f.createInput.ClientIDList)
			assert.Nil(t, f.createInput.ThumbprintList, "ThumbprintList must be omitted so IAM auto-fetches it")
			assert.Contains(t, arn, "oidc-provider/")
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
