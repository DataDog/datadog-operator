package apply

import (
	"context"
	"errors"
	"testing"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	cfntypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	iamtypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/cluster/common/aws"
)

const (
	testIssuerURL   = "https://oidc.eks.eu-west-3.amazonaws.com/id/ABCDEF"
	testProviderArn = "arn:aws:iam::123456789012:oidc-provider/oidc.eks.eu-west-3.amazonaws.com/id/ABCDEF"
)

// fakeOIDCProviderIAM registers at most one OIDC provider, testProviderArn.
type fakeOIDCProviderIAM struct {
	exists    bool
	listErr   error
	listCalls int
}

func (f *fakeOIDCProviderIAM) ListOpenIDConnectProviders(context.Context, *iam.ListOpenIDConnectProvidersInput, ...func(*iam.Options)) (*iam.ListOpenIDConnectProvidersOutput, error) {
	f.listCalls++
	if f.listErr != nil {
		return nil, f.listErr
	}
	out := &iam.ListOpenIDConnectProvidersOutput{}
	if f.exists {
		out.OpenIDConnectProviderList = []iamtypes.OpenIDConnectProviderListEntry{{Arn: awssdk.String(testProviderArn)}}
	}
	return out, nil
}

func (f *fakeOIDCProviderIAM) GetOpenIDConnectProvider(context.Context, *iam.GetOpenIDConnectProviderInput, ...func(*iam.Options)) (*iam.GetOpenIDConnectProviderOutput, error) {
	return &iam.GetOpenIDConnectProviderOutput{ClientIDList: []string{"sts.amazonaws.com"}}, nil
}

func (f *fakeOIDCProviderIAM) AddClientIDToOpenIDConnectProvider(context.Context, *iam.AddClientIDToOpenIDConnectProviderInput, ...func(*iam.Options)) (*iam.AddClientIDToOpenIDConnectProviderOutput, error) {
	return &iam.AddClientIDToOpenIDConnectProviderOutput{}, nil
}

func stackWithParams(params map[string]string) *aws.Stack {
	stack := &cfntypes.Stack{}
	for k, v := range params {
		stack.Parameters = append(stack.Parameters, cfntypes.Parameter{ParameterKey: awssdk.String(k), ParameterValue: awssdk.String(v)})
	}
	return &aws.Stack{Stack: stack}
}

func TestResolveOIDCProvider(t *testing.T) {
	for _, tc := range []struct {
		name           string
		stack          *aws.Stack
		providerExists bool
		listErr        error
		expectArn      string
		expectCreate   bool
		expectLookup   bool
		expectError    string
	}{
		{
			name:         "fresh install on a cluster without provider creates it",
			expectCreate: true,
			expectLookup: true,
		},
		{
			name:           "fresh install on a cluster with a provider reuses it",
			providerExists: true,
			expectArn:      testProviderArn,
			expectLookup:   true,
		},
		{
			name:           "provider created by the stack stays owned by the stack",
			stack:          stackWithParams(map[string]string{"CreateOIDCProvider": "true"}),
			providerExists: true,
			expectCreate:   true,
		},
		{
			name:           "provider reused by the stack keeps being reused",
			stack:          stackWithParams(map[string]string{"CreateOIDCProvider": "false", "OIDCProviderArn": testProviderArn}),
			providerExists: true,
			expectArn:      testProviderArn,
			expectLookup:   true,
		},
		{
			name:           "stack created before CreateOIDCProvider existed reuses the provider",
			stack:          stackWithParams(map[string]string{"OIDCProviderArn": testProviderArn}),
			providerExists: true,
			expectArn:      testProviderArn,
			expectLookup:   true,
		},
		{
			name:         "lookup error propagates",
			listErr:      errors.New("access denied"),
			expectLookup: true,
			expectError:  "failed to look up the OIDC provider",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeOIDCProviderIAM{exists: tc.providerExists, listErr: tc.listErr}

			arn, create, err := resolveOIDCProvider(t.Context(), f, tc.stack, testIssuerURL)

			assert.Equal(t, tc.expectLookup, f.listCalls > 0)
			if tc.expectError != "" {
				require.ErrorContains(t, err, tc.expectError)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.expectArn, arn)
			assert.Equal(t, tc.expectCreate, create)
		})
	}
}
