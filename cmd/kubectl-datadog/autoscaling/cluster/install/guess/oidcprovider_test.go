package guess

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
	providers map[string]string // arn -> url (as stored by IAM, without https://)

	createCalls int
	createURL   string
	createErr   error
	listErr     error
	getErr      error
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
	url, ok := f.providers[aws.ToString(params.OpenIDConnectProviderArn)]
	if !ok {
		return nil, errors.New("not found")
	}
	return &iam.GetOpenIDConnectProviderOutput{Url: aws.String(url)}, nil
}

func (f *fakeIAM) CreateOpenIDConnectProvider(_ context.Context, params *iam.CreateOpenIDConnectProviderInput, _ ...func(*iam.Options)) (*iam.CreateOpenIDConnectProviderOutput, error) {
	f.createCalls++
	f.createURL = aws.ToString(params.Url)
	if f.createErr != nil {
		return nil, f.createErr
	}
	arn := "arn:aws:iam::123456789012:oidc-provider/" + normalizeOIDCURL(f.createURL)
	if f.providers == nil {
		f.providers = map[string]string{}
	}
	f.providers[arn] = normalizeOIDCURL(f.createURL)
	return &iam.CreateOpenIDConnectProviderOutput{OpenIDConnectProviderArn: aws.String(arn)}, nil
}

func TestEnsureOIDCProvider_AlreadyExists(t *testing.T) {
	existingArn := "arn:aws:iam::123456789012:oidc-provider/oidc.eks.eu-west-3.amazonaws.com/id/ABCDEF"
	f := &fakeIAM{
		providers: map[string]string{
			existingArn: "oidc.eks.eu-west-3.amazonaws.com/id/ABCDEF",
		},
	}

	arn, err := EnsureOIDCProvider(context.Background(), f, "https://oidc.eks.eu-west-3.amazonaws.com/id/ABCDEF")

	require.NoError(t, err)
	assert.Equal(t, existingArn, arn)
	assert.Equal(t, 0, f.createCalls, "should not create when provider already exists")
}

func TestEnsureOIDCProvider_CaseInsensitiveHostMatch(t *testing.T) {
	existingArn := "arn:aws:iam::123456789012:oidc-provider/oidc.eks.eu-west-3.amazonaws.com/id/ABCDEF"
	f := &fakeIAM{
		providers: map[string]string{
			existingArn: "oidc.eks.eu-west-3.amazonaws.com/id/ABCDEF",
		},
	}

	// Scheme and host are case-insensitive per RFC 3986.
	arn, err := EnsureOIDCProvider(context.Background(), f, "HTTPS://OIDC.EKS.EU-WEST-3.AMAZONAWS.COM/id/ABCDEF")

	require.NoError(t, err)
	assert.Equal(t, existingArn, arn)
	assert.Equal(t, 0, f.createCalls)
}

func TestEnsureOIDCProvider_PathIsCaseSensitive(t *testing.T) {
	// Two providers whose URLs differ only by path case must not be treated as
	// equal: the path is case-sensitive per RFC 3986.
	existingArn := "arn:aws:iam::123456789012:oidc-provider/oidc.eks.eu-west-3.amazonaws.com/id/abcdef"
	f := &fakeIAM{
		providers: map[string]string{
			existingArn: "oidc.eks.eu-west-3.amazonaws.com/id/abcdef",
		},
	}

	arn, err := EnsureOIDCProvider(context.Background(), f, "https://oidc.eks.eu-west-3.amazonaws.com/id/ABCDEF")

	require.NoError(t, err)
	assert.NotEqual(t, existingArn, arn)
	assert.Equal(t, 1, f.createCalls)
}

func TestEnsureOIDCProvider_CreatesWhenMissing(t *testing.T) {
	f := &fakeIAM{providers: map[string]string{}}

	arn, err := EnsureOIDCProvider(context.Background(), f, "https://oidc.eks.eu-west-3.amazonaws.com/id/NEW")

	require.NoError(t, err)
	assert.Equal(t, 1, f.createCalls)
	assert.Equal(t, "https://oidc.eks.eu-west-3.amazonaws.com/id/NEW", f.createURL)
	assert.Contains(t, arn, "oidc-provider/")
}

func TestEnsureOIDCProvider_CreatesWhenListEmpty(t *testing.T) {
	f := &fakeIAM{}

	_, err := EnsureOIDCProvider(context.Background(), f, "https://oidc.eks.eu-west-3.amazonaws.com/id/XYZ")

	require.NoError(t, err)
	assert.Equal(t, 1, f.createCalls)
}

func TestEnsureOIDCProvider_ListError(t *testing.T) {
	f := &fakeIAM{listErr: errors.New("api throttled")}

	_, err := EnsureOIDCProvider(context.Background(), f, "https://oidc.eks.eu-west-3.amazonaws.com/id/XYZ")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to list OIDC providers")
}

func TestEnsureOIDCProvider_CreateError(t *testing.T) {
	f := &fakeIAM{
		providers: map[string]string{},
		createErr: errors.New("quota exceeded"),
	}

	_, err := EnsureOIDCProvider(context.Background(), f, "https://oidc.eks.eu-west-3.amazonaws.com/id/XYZ")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create OIDC provider")
}

func TestEnsureOIDCProvider_CreateRequestsSTSClientID(t *testing.T) {
	var capturedInput *iam.CreateOpenIDConnectProviderInput
	captureIAM := &captureCreateIAM{inner: &fakeIAM{}}
	captureIAM.capture = &capturedInput

	_, err := EnsureOIDCProvider(context.Background(), captureIAM, "https://oidc.eks.eu-west-3.amazonaws.com/id/XYZ")
	require.NoError(t, err)

	require.NotNil(t, capturedInput)
	assert.Equal(t, []string{"sts.amazonaws.com"}, capturedInput.ClientIDList)
	assert.Nil(t, capturedInput.ThumbprintList, "ThumbprintList must be omitted so IAM auto-fetches it")
}

type captureCreateIAM struct {
	inner   *fakeIAM
	capture **iam.CreateOpenIDConnectProviderInput
}

func (c *captureCreateIAM) ListOpenIDConnectProviders(ctx context.Context, params *iam.ListOpenIDConnectProvidersInput, optFns ...func(*iam.Options)) (*iam.ListOpenIDConnectProvidersOutput, error) {
	return c.inner.ListOpenIDConnectProviders(ctx, params, optFns...)
}

func (c *captureCreateIAM) GetOpenIDConnectProvider(ctx context.Context, params *iam.GetOpenIDConnectProviderInput, optFns ...func(*iam.Options)) (*iam.GetOpenIDConnectProviderOutput, error) {
	return c.inner.GetOpenIDConnectProvider(ctx, params, optFns...)
}

func (c *captureCreateIAM) CreateOpenIDConnectProvider(ctx context.Context, params *iam.CreateOpenIDConnectProviderInput, optFns ...func(*iam.Options)) (*iam.CreateOpenIDConnectProviderOutput, error) {
	*c.capture = params
	return c.inner.CreateOpenIDConnectProvider(ctx, params, optFns...)
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
