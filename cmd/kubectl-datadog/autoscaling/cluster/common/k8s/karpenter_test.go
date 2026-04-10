package k8s

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
)

func TestDetectActiveKarpenter(t *testing.T) {
	for _, tc := range []struct {
		name              string
		objects           []runtime.Object
		expectedFound     bool
		expectedNamespace string
	}{
		{
			name:          "No webhook configurations",
			objects:       nil,
			expectedFound: false,
		},
		{
			name: "Karpenter ValidatingWebhookConfiguration in dd-karpenter",
			objects: []runtime.Object{
				&admissionregistrationv1.ValidatingWebhookConfiguration{
					ObjectMeta: metav1.ObjectMeta{Name: "karpenter-validation"},
					Webhooks: []admissionregistrationv1.ValidatingWebhook{
						{
							Name: "validation.karpenter.sh",
							Rules: []admissionregistrationv1.RuleWithOperations{
								{
									Rule: admissionregistrationv1.Rule{
										APIGroups: []string{"karpenter.sh"},
									},
								},
							},
							ClientConfig: admissionregistrationv1.WebhookClientConfig{
								Service: &admissionregistrationv1.ServiceReference{
									Namespace: "dd-karpenter",
									Name:      "karpenter",
								},
							},
						},
					},
				},
			},
			expectedFound:     true,
			expectedNamespace: "dd-karpenter",
		},
		{
			name: "Karpenter ValidatingWebhookConfiguration in custom namespace",
			objects: []runtime.Object{
				&admissionregistrationv1.ValidatingWebhookConfiguration{
					ObjectMeta: metav1.ObjectMeta{Name: "karpenter-validation"},
					Webhooks: []admissionregistrationv1.ValidatingWebhook{
						{
							Name: "validation.karpenter.k8s.aws",
							Rules: []admissionregistrationv1.RuleWithOperations{
								{
									Rule: admissionregistrationv1.Rule{
										APIGroups: []string{"karpenter.k8s.aws"},
									},
								},
							},
							ClientConfig: admissionregistrationv1.WebhookClientConfig{
								Service: &admissionregistrationv1.ServiceReference{
									Namespace: "karpenter",
									Name:      "karpenter",
								},
							},
						},
					},
				},
			},
			expectedFound:     true,
			expectedNamespace: "karpenter",
		},
		{
			name: "Non-Karpenter ValidatingWebhookConfiguration only",
			objects: []runtime.Object{
				&admissionregistrationv1.ValidatingWebhookConfiguration{
					ObjectMeta: metav1.ObjectMeta{Name: "other-webhook"},
					Webhooks: []admissionregistrationv1.ValidatingWebhook{
						{
							Name: "validate.something.io",
							Rules: []admissionregistrationv1.RuleWithOperations{
								{
									Rule: admissionregistrationv1.Rule{
										APIGroups: []string{"apps"},
									},
								},
							},
							ClientConfig: admissionregistrationv1.WebhookClientConfig{
								Service: &admissionregistrationv1.ServiceReference{
									Namespace: "system",
									Name:      "webhook-svc",
								},
							},
						},
					},
				},
			},
			expectedFound: false,
		},
		{
			name: "Karpenter MutatingWebhookConfiguration detected",
			objects: []runtime.Object{
				&admissionregistrationv1.MutatingWebhookConfiguration{
					ObjectMeta: metav1.ObjectMeta{Name: "karpenter-defaulting"},
					Webhooks: []admissionregistrationv1.MutatingWebhook{
						{
							Name: "defaulting.karpenter.sh",
							Rules: []admissionregistrationv1.RuleWithOperations{
								{
									Rule: admissionregistrationv1.Rule{
										APIGroups: []string{"karpenter.sh"},
									},
								},
							},
							ClientConfig: admissionregistrationv1.WebhookClientConfig{
								Service: &admissionregistrationv1.ServiceReference{
									Namespace: "dd-karpenter",
									Name:      "karpenter",
								},
							},
						},
					},
				},
			},
			expectedFound:     true,
			expectedNamespace: "dd-karpenter",
		},
		{
			name: "Karpenter webhook with URL-based config (no service)",
			objects: []runtime.Object{
				&admissionregistrationv1.ValidatingWebhookConfiguration{
					ObjectMeta: metav1.ObjectMeta{Name: "karpenter-validation"},
					Webhooks: []admissionregistrationv1.ValidatingWebhook{
						{
							Name: "validation.karpenter.sh",
							Rules: []admissionregistrationv1.RuleWithOperations{
								{
									Rule: admissionregistrationv1.Rule{
										APIGroups: []string{"karpenter.sh"},
									},
								},
							},
							ClientConfig: admissionregistrationv1.WebhookClientConfig{
								URL: ptrString("https://external-karpenter.example.com/validate"),
							},
						},
					},
				},
			},
			expectedFound:     true,
			expectedNamespace: "",
		},
		{
			name: "Multiple webhooks, only one is Karpenter",
			objects: []runtime.Object{
				&admissionregistrationv1.ValidatingWebhookConfiguration{
					ObjectMeta: metav1.ObjectMeta{Name: "other-webhook"},
					Webhooks: []admissionregistrationv1.ValidatingWebhook{
						{
							Name: "validate.other.io",
							Rules: []admissionregistrationv1.RuleWithOperations{
								{
									Rule: admissionregistrationv1.Rule{
										APIGroups: []string{"apps"},
									},
								},
							},
							ClientConfig: admissionregistrationv1.WebhookClientConfig{
								Service: &admissionregistrationv1.ServiceReference{
									Namespace: "other",
									Name:      "other-svc",
								},
							},
						},
					},
				},
				&admissionregistrationv1.ValidatingWebhookConfiguration{
					ObjectMeta: metav1.ObjectMeta{Name: "karpenter-validation"},
					Webhooks: []admissionregistrationv1.ValidatingWebhook{
						{
							Name: "validation.karpenter.sh",
							Rules: []admissionregistrationv1.RuleWithOperations{
								{
									Rule: admissionregistrationv1.Rule{
										APIGroups: []string{"karpenter.sh"},
									},
								},
							},
							ClientConfig: admissionregistrationv1.WebhookClientConfig{
								Service: &admissionregistrationv1.ServiceReference{
									Namespace: "dd-karpenter",
									Name:      "karpenter",
								},
							},
						},
					},
				},
			},
			expectedFound:     true,
			expectedNamespace: "dd-karpenter",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			clientset := fake.NewSimpleClientset(tc.objects...)

			found, ns, err := DetectActiveKarpenter(context.Background(), clientset)
			assert.NoError(t, err)
			assert.Equal(t, tc.expectedFound, found)
			assert.Equal(t, tc.expectedNamespace, ns)
		})
	}
}

func TestFindKarpenterHelmRelease(t *testing.T) {
	for _, tc := range []struct {
		name               string
		objects            []runtime.Object
		expectedFound      bool
		expectedNamespaces []string
	}{
		{
			name:          "No Helm secrets",
			objects:       nil,
			expectedFound: false,
		},
		{
			name: "Karpenter Helm secret in dd-karpenter",
			objects: []runtime.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "sh.helm.release.v1.karpenter.v1",
						Namespace: "dd-karpenter",
						Labels: map[string]string{
							"owner":  "helm",
							"name":   "karpenter",
							"status": "deployed",
						},
					},
				},
			},
			expectedFound:      true,
			expectedNamespaces: []string{"dd-karpenter"},
		},
		{
			name: "Karpenter Helm secret in custom namespace",
			objects: []runtime.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "sh.helm.release.v1.karpenter.v3",
						Namespace: "my-karpenter",
						Labels: map[string]string{
							"owner":  "helm",
							"name":   "karpenter",
							"status": "deployed",
						},
					},
				},
			},
			expectedFound:      true,
			expectedNamespaces: []string{"my-karpenter"},
		},
		{
			name: "Karpenter Helm secret with superseded status (not deployed)",
			objects: []runtime.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "sh.helm.release.v1.karpenter.v1",
						Namespace: "dd-karpenter",
						Labels: map[string]string{
							"owner":  "helm",
							"name":   "karpenter",
							"status": "superseded",
						},
					},
				},
			},
			expectedFound: false,
		},
		{
			name: "Different Helm release (not karpenter)",
			objects: []runtime.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "sh.helm.release.v1.datadog.v1",
						Namespace: "datadog",
						Labels: map[string]string{
							"owner":  "helm",
							"name":   "datadog",
							"status": "deployed",
						},
					},
				},
			},
			expectedFound: false,
		},
		{
			name: "Multiple revisions in same namespace are deduplicated",
			objects: []runtime.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "sh.helm.release.v1.karpenter.v1",
						Namespace: "dd-karpenter",
						Labels: map[string]string{
							"owner":  "helm",
							"name":   "karpenter",
							"status": "deployed",
						},
					},
				},
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "sh.helm.release.v1.karpenter.v2",
						Namespace: "dd-karpenter",
						Labels: map[string]string{
							"owner":  "helm",
							"name":   "karpenter",
							"status": "deployed",
						},
					},
				},
			},
			expectedFound:      true,
			expectedNamespaces: []string{"dd-karpenter"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			clientset := fake.NewSimpleClientset(tc.objects...)

			found, namespaces, err := FindKarpenterHelmRelease(context.Background(), clientset)
			assert.NoError(t, err)
			assert.Equal(t, tc.expectedFound, found)
			if tc.expectedNamespaces == nil {
				assert.Nil(t, namespaces)
			} else {
				assert.ElementsMatch(t, tc.expectedNamespaces, namespaces)
			}
		})
	}
}

func ptrString(s string) *string {
	return &s
}
