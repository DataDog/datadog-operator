package eks

import (
	"context"
	"errors"
	"net/url"
	"strconv"
	"strings"
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
	tags         []iamtypes.Tag
}

type fakeIAM struct {
	providers map[string]*fakeProvider // arn -> provider
	roles     []iamtypes.Role

	createCalls       int
	createInput       *iam.CreateOpenIDConnectProviderInput // captured on Create
	createErr         error
	listErr           error
	getErr            error
	addClientIDCalls  int
	addClientIDInputs []*iam.AddClientIDToOpenIDConnectProviderInput
	deleteCalls       int
	deleteErr         error
	listRolesErr      error
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
		Tags:         p.tags,
	}, nil
}

func (f *fakeIAM) DeleteOpenIDConnectProvider(_ context.Context, params *iam.DeleteOpenIDConnectProviderInput, _ ...func(*iam.Options)) (*iam.DeleteOpenIDConnectProviderOutput, error) {
	f.deleteCalls++
	if f.deleteErr != nil {
		return nil, f.deleteErr
	}
	delete(f.providers, aws.ToString(params.OpenIDConnectProviderArn))
	return &iam.DeleteOpenIDConnectProviderOutput{}, nil
}

// ListRoles returns one role per page to exercise pagination.
func (f *fakeIAM) ListRoles(_ context.Context, params *iam.ListRolesInput, _ ...func(*iam.Options)) (*iam.ListRolesOutput, error) {
	if f.listRolesErr != nil {
		return nil, f.listRolesErr
	}
	i := 0
	if params.Marker != nil {
		i, _ = strconv.Atoi(aws.ToString(params.Marker))
	}
	if i >= len(f.roles) {
		return &iam.ListRolesOutput{}, nil
	}
	out := &iam.ListRolesOutput{Roles: f.roles[i : i+1]}
	if i+1 < len(f.roles) {
		out.IsTruncated = true
		out.Marker = aws.String(strconv.Itoa(i + 1))
	}
	return out, nil
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
	f.providers[arn] = &fakeProvider{url: normalized, clientIDList: params.ClientIDList, tags: params.Tags}
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
			assert.Equal(t, []iamtypes.Tag{managedByTag}, f.createInput.Tags, "created provider must be tagged so that uninstall can delete it")
			assert.Contains(t, arn, "oidc-provider/")
		})
	}
}

var managedByTag = iamtypes.Tag{Key: aws.String("managed-by"), Value: aws.String("kubectl-datadog")}

// trustPolicy returns the URL-encoded trust policy of an IRSA role trusting
// the given OIDC provider, as returned by IAM ListRoles.
func trustPolicy(providerArn string) *string {
	return aws.String(url.QueryEscape(`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Federated":"` + providerArn + `"},"Action":"sts:AssumeRoleWithWebIdentity"}]}`))
}

func TestDeleteOwnedOIDCProvider(t *testing.T) {
	const (
		issuerURL   = "https://oidc.eks.eu-west-3.amazonaws.com/id/ABCDEF"
		issuerStore = "oidc.eks.eu-west-3.amazonaws.com/id/ABCDEF"
		issuerArn   = "arn:aws:iam::123456789012:oidc-provider/" + issuerStore
		otherArn    = "arn:aws:iam::123456789012:oidc-provider/oidc.eks.eu-west-3.amazonaws.com/id/OTHER"
	)

	// Spaces in a trust policy may be URL-encoded as either `%20` or `+`.
	multiLinePolicy := "{\n  \"Principal\": { \"Federated\": \"" + issuerArn + "\" }\n}"

	unrelatedRole := iamtypes.Role{RoleName: aws.String("unrelated"), AssumeRolePolicyDocument: trustPolicy(otherArn)}

	// owned returns a fresh provider set, as the fake's Delete mutates it.
	owned := func() map[string]*fakeProvider {
		return map[string]*fakeProvider{issuerArn: {url: issuerStore, tags: []iamtypes.Tag{managedByTag}}}
	}

	for _, tc := range []struct {
		name          string
		existing      map[string]*fakeProvider
		roles         []iamtypes.Role
		listErr       error
		getErr        error
		listRolesErr  error
		deleteErr     error
		expectDeleted bool
		expectSkip    string
		expectError   string
	}{
		{
			name:          "owned and untrusted provider is deleted",
			existing:      owned(),
			roles:         []iamtypes.Role{unrelatedRole, unrelatedRole},
			expectDeleted: true,
		},
		{
			name:       "no provider for the issuer",
			existing:   map[string]*fakeProvider{otherArn: {url: "oidc.eks.eu-west-3.amazonaws.com/id/OTHER", tags: []iamtypes.Tag{managedByTag}}},
			expectSkip: "no OIDC provider is registered",
		},
		{
			name:       "untagged provider is not deleted",
			existing:   map[string]*fakeProvider{issuerArn: {url: issuerStore}},
			expectSkip: "was not created by kubectl-datadog",
		},
		{
			name: "provider tagged by another tool is not deleted",
			existing: map[string]*fakeProvider{issuerArn: {url: issuerStore, tags: []iamtypes.Tag{
				{Key: aws.String("managed-by"), Value: aws.String("eksctl")},
			}}},
			expectSkip: "was not created by kubectl-datadog",
		},
		{
			name:     "own IRSA role still listed after its deletion is ignored",
			existing: owned(),
			roles: []iamtypes.Role{
				{RoleName: aws.String("my-cluster-karpenter"), AssumeRolePolicyDocument: trustPolicy(issuerArn)},
			},
			expectDeleted: true,
		},
		{
			name:     "provider still trusted by a role is not deleted",
			existing: owned(),
			roles: []iamtypes.Role{
				unrelatedRole,
				{RoleName: aws.String("aws-load-balancer-controller"), AssumeRolePolicyDocument: trustPolicy(issuerArn)},
			},
			expectSkip: "is still trusted by IAM role(s) aws-load-balancer-controller",
		},
		{
			name:     "provider trusted through a policy with escaped slashes is not deleted",
			existing: owned(),
			roles: []iamtypes.Role{
				{RoleName: aws.String("escaped"), AssumeRolePolicyDocument: trustPolicy(strings.ReplaceAll(issuerArn, "/", `\/`))},
			},
			expectSkip: "is still trusted by IAM role(s) escaped",
		},
		{
			name:     "provider whose ARN is a prefix of a trusted one is deleted",
			existing: owned(),
			roles: []iamtypes.Role{
				{RoleName: aws.String("other"), AssumeRolePolicyDocument: trustPolicy(issuerArn + "0")},
			},
			expectDeleted: true,
		},
		{
			name:     "provider trusted through a multi-line policy is not deleted",
			existing: owned(),
			roles: []iamtypes.Role{
				{RoleName: aws.String("percent-encoded"), AssumeRolePolicyDocument: aws.String(url.PathEscape(multiLinePolicy))},
				{RoleName: aws.String("plus-encoded"), AssumeRolePolicyDocument: aws.String(url.QueryEscape(multiLinePolicy))},
			},
			expectSkip: "is still trusted by IAM role(s) percent-encoded, plus-encoded",
		},
		{
			name:     "provider is kept when a trust policy cannot be decoded",
			existing: owned(),
			roles: []iamtypes.Role{
				unrelatedRole,
				{RoleName: aws.String("undecodable"), AssumeRolePolicyDocument: aws.String("%7Bnot-json")},
			},
			expectSkip: "is still trusted by IAM role(s) undecodable",
		},
		{
			name:       "provider deleted concurrently while being looked up",
			existing:   owned(),
			getErr:     &iamtypes.NoSuchEntityException{},
			expectSkip: "no OIDC provider is registered",
		},
		{
			name:        "list providers error propagates",
			listErr:     errors.New("api throttled"),
			expectError: "failed to list OIDC providers",
		},
		{
			name:         "list roles error propagates",
			existing:     owned(),
			listRolesErr: errors.New("access denied"),
			expectError:  "failed to list IAM roles",
		},
		{
			name:       "provider deleted concurrently",
			existing:   owned(),
			deleteErr:  &iamtypes.NoSuchEntityException{},
			expectSkip: "is already deleted",
		},
		{
			name:        "delete error propagates",
			existing:    owned(),
			deleteErr:   errors.New("access denied"),
			expectError: "failed to delete OIDC provider",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeIAM{
				providers:    tc.existing,
				roles:        tc.roles,
				listErr:      tc.listErr,
				getErr:       tc.getErr,
				listRolesErr: tc.listRolesErr,
				deleteErr:    tc.deleteErr,
			}

			deletedArn, skipReason, err := DeleteOwnedOIDCProvider(t.Context(), f, issuerURL, "my-cluster-karpenter")

			wantDeleteCalls := 0
			if tc.expectDeleted || tc.deleteErr != nil {
				wantDeleteCalls = 1
			}
			assert.Equal(t, wantDeleteCalls, f.deleteCalls, "a provider that is not owned or still trusted must not be deleted")

			if tc.expectError != "" {
				require.ErrorContains(t, err, tc.expectError)
				return
			}
			require.NoError(t, err)

			if tc.expectDeleted {
				assert.Equal(t, issuerArn, deletedArn)
				assert.Empty(t, skipReason)
				assert.NotContains(t, f.providers, issuerArn)
				return
			}
			assert.Empty(t, deletedArn)
			assert.Contains(t, skipReason, tc.expectSkip)
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
