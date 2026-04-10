package clients

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateAccountIDs(t *testing.T) {
	tests := []struct {
		name                 string
		credentialsAccountID string
		clusterARN           string
		clusterName          string
		wantErr              bool
		wantMismatch         bool
		errContains          string
	}{
		{
			name:                 "matching accounts",
			credentialsAccountID: "123456789012",
			clusterARN:           "arn:aws:eks:us-east-1:123456789012:cluster/my-cluster",
			clusterName:          "my-cluster",
		},
		{
			name:                 "mismatched accounts",
			credentialsAccountID: "111111111111",
			clusterARN:           "arn:aws:eks:us-east-1:222222222222:cluster/my-cluster",
			clusterName:          "my-cluster",
			wantErr:              true,
			wantMismatch:         true,
			errContains:          "AWS account mismatch",
		},
		{
			name:                 "matching accounts in GovCloud partition",
			credentialsAccountID: "123456789012",
			clusterARN:           "arn:aws-us-gov:eks:us-gov-west-1:123456789012:cluster/my-cluster",
			clusterName:          "my-cluster",
		},
		{
			name:                 "matching accounts in China partition",
			credentialsAccountID: "123456789012",
			clusterARN:           "arn:aws-cn:eks:cn-north-1:123456789012:cluster/my-cluster",
			clusterName:          "my-cluster",
		},
		{
			name:                 "invalid ARN format",
			credentialsAccountID: "123456789012",
			clusterARN:           "not-an-arn",
			clusterName:          "my-cluster",
			wantErr:              true,
			errContains:          "failed to parse EKS cluster ARN",
		},
		{
			name:                 "error message contains both account IDs",
			credentialsAccountID: "111111111111",
			clusterARN:           "arn:aws:eks:eu-west-1:999999999999:cluster/prod-cluster",
			clusterName:          "prod-cluster",
			wantErr:              true,
			wantMismatch:         true,
			errContains:          "999999999999",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateAccountIDs(tt.credentialsAccountID, tt.clusterARN, tt.clusterName)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)

				var mismatch *AccountMismatchError
				if tt.wantMismatch {
					assert.True(t, errors.As(err, &mismatch))
				} else {
					assert.False(t, errors.As(err, &mismatch))
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
