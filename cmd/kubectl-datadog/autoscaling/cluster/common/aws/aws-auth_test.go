package aws

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestEnsureAwsAuthRole(t *testing.T) {
	for _, tc := range []struct {
		name          string
		existingRoles []RoleMapping
		newRole       RoleMapping
		expectError   bool
		expectUpdate  bool
		expectedRoles int
	}{
		{
			name:          "Add role to empty ConfigMap",
			existingRoles: []RoleMapping{},
			newRole: RoleMapping{
				RoleArn:  "arn:aws:iam::123456789012:role/TestRole",
				Username: "system:node:{{EC2PrivateDNSName}}",
				Groups:   []string{"system:bootstrappers", "system:nodes"},
			},
			expectError:   false,
			expectUpdate:  true,
			expectedRoles: 1,
		},
		{
			name: "Add role to existing ConfigMap",
			existingRoles: []RoleMapping{
				{
					RoleArn:  "arn:aws:iam::123456789012:role/ExistingRole",
					Username: "existing-user",
					Groups:   []string{"existing-group"},
				},
			},
			newRole: RoleMapping{
				RoleArn:  "arn:aws:iam::123456789012:role/TestRole",
				Username: "system:node:{{EC2PrivateDNSName}}",
				Groups:   []string{"system:bootstrappers", "system:nodes"},
			},
			expectError:   false,
			expectUpdate:  true,
			expectedRoles: 2,
		},
		{
			name: "Role already exists - no update",
			existingRoles: []RoleMapping{
				{
					RoleArn:  "arn:aws:iam::123456789012:role/TestRole",
					Username: "system:node:{{EC2PrivateDNSName}}",
					Groups:   []string{"system:bootstrappers", "system:nodes"},
				},
			},
			newRole: RoleMapping{
				RoleArn:  "arn:aws:iam::123456789012:role/TestRole",
				Username: "system:node:{{EC2PrivateDNSName}}",
				Groups:   []string{"system:bootstrappers", "system:nodes"},
			},
			expectError:   false,
			expectUpdate:  false,
			expectedRoles: 1,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			clientset := fake.NewSimpleClientset()

			existingRolesYAML, err := yaml.Marshal(tc.existingRoles)
			require.NoError(t, err)

			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "aws-auth",
					Namespace: "kube-system",
				},
				Data: map[string]string{},
			}

			if len(tc.existingRoles) > 0 {
				cm.Data["mapRoles"] = string(existingRolesYAML)
			}

			_, err = clientset.CoreV1().ConfigMaps("kube-system").Create(t.Context(), cm, metav1.CreateOptions{})
			require.NoError(t, err)

			err = EnsureAwsAuthRole(t.Context(), clientset, tc.newRole)

			if tc.expectError {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)

			updatedCM, err := clientset.CoreV1().ConfigMaps("kube-system").Get(t.Context(), "aws-auth", metav1.GetOptions{})
			require.NoError(t, err)

			var updatedRoles []RoleMapping
			if mapRoles, ok := updatedCM.Data["mapRoles"]; ok {
				err = yaml.Unmarshal([]byte(mapRoles), &updatedRoles)
				require.NoError(t, err)
			}

			assert.Equal(t, tc.expectedRoles, len(updatedRoles))

			found := false
			for _, role := range updatedRoles {
				if role.RoleArn == tc.newRole.RoleArn {
					found = true
					assert.Equal(t, tc.newRole.Username, role.Username)
					assert.Equal(t, tc.newRole.Groups, role.Groups)
					break
				}
			}
			assert.True(t, found, "New role should be present in the ConfigMap")
		})
	}
}

func TestEnsureAwsAuthRole_ConfigMapNotFound(t *testing.T) {
	clientset := fake.NewSimpleClientset()

	roleMapping := RoleMapping{
		RoleArn:  "arn:aws:iam::123456789012:role/TestRole",
		Username: "system:node:{{EC2PrivateDNSName}}",
		Groups:   []string{"system:bootstrappers", "system:nodes"},
	}

	err := EnsureAwsAuthRole(t.Context(), clientset, roleMapping)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get aws-auth ConfigMap")
}
