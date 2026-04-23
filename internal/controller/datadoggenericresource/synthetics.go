package datadoggenericresource

import (
	"context"
	"encoding/json"
	"time"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
)

type SyntheticsAPITestHandler struct {
	auth   context.Context
	client *datadogV1.SyntheticsApi
}

func (h *SyntheticsAPITestHandler) createResource(instance *v1alpha1.DatadogGenericResource) (CreateResult, error) {
	createdTest, err := createSyntheticsAPITest(h.auth, h.client, instance)
	if err != nil {
		return CreateResult{}, err
	}
	return createResultFromSyntheticsTest(&createdTest, createdTest.AdditionalProperties), nil
}

func (h *SyntheticsAPITestHandler) getResource(instance *v1alpha1.DatadogGenericResource) error {
	_, err := getSyntheticsTest(h.auth, h.client, instance.Status.Id)
	return err
}
func (h *SyntheticsAPITestHandler) updateResource(instance *v1alpha1.DatadogGenericResource) error {
	_, err := updateSyntheticsAPITest(h.auth, h.client, instance)
	return err
}
func (h *SyntheticsAPITestHandler) deleteResource(instance *v1alpha1.DatadogGenericResource) error {
	return deleteSyntheticTest(h.auth, h.client, instance.Status.Id)
}

type SyntheticsBrowserTestHandler struct {
	auth   context.Context
	client *datadogV1.SyntheticsApi
}

func (h *SyntheticsBrowserTestHandler) createResource(instance *v1alpha1.DatadogGenericResource) (CreateResult, error) {
	createdTest, err := createSyntheticBrowserTest(h.auth, h.client, instance)
	if err != nil {
		return CreateResult{}, err
	}
	return createResultFromSyntheticsTest(&createdTest, createdTest.AdditionalProperties), nil
}

func (h *SyntheticsBrowserTestHandler) getResource(instance *v1alpha1.DatadogGenericResource) error {
	_, err := getSyntheticsTest(h.auth, h.client, instance.Status.Id)
	return err
}
func (h *SyntheticsBrowserTestHandler) updateResource(instance *v1alpha1.DatadogGenericResource) error {
	_, err := updateSyntheticsBrowserTest(h.auth, h.client, instance)
	return err
}
func (h *SyntheticsBrowserTestHandler) deleteResource(instance *v1alpha1.DatadogGenericResource) error {
	return deleteSyntheticTest(h.auth, h.client, instance.Status.Id)
}

// createResultFromSyntheticsTest extracts the common fields from a synthetic test API response.
func createResultFromSyntheticsTest(createdTest interface{ GetPublicId() string }, additionalProperties map[string]any) CreateResult {
	var createdTime *metav1.Time
	if createdTimeString, ok := additionalProperties["created_at"].(string); ok {
		if t, err := time.Parse(time.RFC3339, createdTimeString); err == nil {
			ct := metav1.NewTime(t)
			createdTime = &ct
		}
	}

	creator := ""
	if createdBy, ok := additionalProperties["created_by"].(map[string]any); ok {
		if handle, ok := createdBy["handle"].(string); ok {
			creator = handle
		}
	}

	return CreateResult{
		ID:          createdTest.GetPublicId(),
		CreatedTime: createdTime,
		Creator:     creator,
	}
}

// Synthetic tests (encompass browser and API tests): get
func getSyntheticsTest(auth context.Context, client *datadogV1.SyntheticsApi, testID string) (datadogV1.SyntheticsTestDetailsWithoutSteps, error) {
	test, _, err := client.GetTest(auth, testID)
	if err != nil {
		return datadogV1.SyntheticsTestDetailsWithoutSteps{}, translateClientError(err, "error getting synthetic test")
	}
	return test, nil
}

// Synthetic tests (encompass browser and API tests): delete
func deleteSyntheticTest(auth context.Context, client *datadogV1.SyntheticsApi, ID string) error {
	body := datadogV1.SyntheticsDeleteTestsPayload{
		PublicIds: []string{
			ID,
		},
	}
	_, httpResponse, err := client.DeleteTests(auth, body)
	if err != nil {
		// Deletion is idempotent for finalization: if the synthetic test was already
		// removed in Datadog (for example from the UI), allow the Kubernetes finalizer
		// to clear. Retry other errors (e.g. 400, 401, 429, 5XX).
		if httpResponse != nil && httpResponse.StatusCode == 404 {
			return nil
		}
		return translateClientError(err, "error deleting synthetic test")
	}
	return nil
}

// Browser test: create
func createSyntheticBrowserTest(auth context.Context, client *datadogV1.SyntheticsApi, instance *v1alpha1.DatadogGenericResource) (datadogV1.SyntheticsBrowserTest, error) {
	browserTestBody := &datadogV1.SyntheticsBrowserTest{}
	if err := json.Unmarshal([]byte(instance.Spec.JsonSpec), browserTestBody); err != nil {
		return datadogV1.SyntheticsBrowserTest{}, translateClientError(err, "error unmarshalling browser test spec")
	}
	test, _, err := client.CreateSyntheticsBrowserTest(auth, *browserTestBody)
	if err != nil {
		return datadogV1.SyntheticsBrowserTest{}, translateClientError(err, "error creating browser test")
	}
	return test, nil
}

// Browser test: update
func updateSyntheticsBrowserTest(auth context.Context, client *datadogV1.SyntheticsApi, instance *v1alpha1.DatadogGenericResource) (datadogV1.SyntheticsBrowserTest, error) {
	browserTestBody := &datadogV1.SyntheticsBrowserTest{}
	if err := json.Unmarshal([]byte(instance.Spec.JsonSpec), browserTestBody); err != nil {
		return datadogV1.SyntheticsBrowserTest{}, translateClientError(err, "error unmarshalling browser test spec")
	}
	testUpdated, _, err := client.UpdateBrowserTest(auth, instance.Status.Id, *browserTestBody)
	if err != nil {
		return datadogV1.SyntheticsBrowserTest{}, translateClientError(err, "error updating browser test")
	}
	return testUpdated, nil
}

// API test: create
func createSyntheticsAPITest(auth context.Context, client *datadogV1.SyntheticsApi, instance *v1alpha1.DatadogGenericResource) (datadogV1.SyntheticsAPITest, error) {
	apiTestBody := &datadogV1.SyntheticsAPITest{}
	if err := json.Unmarshal([]byte(instance.Spec.JsonSpec), apiTestBody); err != nil {
		return datadogV1.SyntheticsAPITest{}, translateClientError(err, "error unmarshalling API test spec")
	}
	test, _, err := client.CreateSyntheticsAPITest(auth, *apiTestBody)
	if err != nil {
		return datadogV1.SyntheticsAPITest{}, translateClientError(err, "error creating API test")
	}
	return test, nil
}

// API test: update
func updateSyntheticsAPITest(auth context.Context, client *datadogV1.SyntheticsApi, instance *v1alpha1.DatadogGenericResource) (datadogV1.SyntheticsAPITest, error) {
	apiTestBody := &datadogV1.SyntheticsAPITest{}
	if err := json.Unmarshal([]byte(instance.Spec.JsonSpec), apiTestBody); err != nil {
		return datadogV1.SyntheticsAPITest{}, translateClientError(err, "error unmarshalling API test spec")
	}
	testUpdated, _, err := client.UpdateAPITest(auth, instance.Status.Id, *apiTestBody)
	if err != nil {
		return datadogV1.SyntheticsAPITest{}, translateClientError(err, "error updating API test")
	}
	return testUpdated, nil
}
