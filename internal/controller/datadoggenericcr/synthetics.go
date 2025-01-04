package datadoggenericcr

import (
	"context"
	"encoding/json"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
)

type SyntheticsAPITestHandler struct{}

func (h *SyntheticsAPITestHandler) createResourcefunc(r *Reconciler, logger logr.Logger, instance *v1alpha1.DatadogGenericCR, status *v1alpha1.DatadogGenericCRStatus, now metav1.Time, hash string) error {
	createdTest, err := createSyntheticBrowserTest(r.datadogAuth, r.datadogSyntheticsClient, instance)
	if err != nil {
		logger.Error(err, "error creating browser test")
		updateErrStatus(status, now, v1alpha1.DatadogSyncStatusCreateError, "CreatingCustomResource", err)
		return err
	}
	additionalProperties := createdTest.AdditionalProperties
	return updateStatusFromSyntheticsTest(&createdTest, additionalProperties, status, logger, hash)
}

func (h *SyntheticsAPITestHandler) getResourcefunc(r *Reconciler, instance *v1alpha1.DatadogGenericCR) error {
	_, err := getSyntheticsTest(r.datadogAuth, r.datadogSyntheticsClient, instance.Status.Id)
	return err
}
func (h *SyntheticsAPITestHandler) updateResourcefunc(r *Reconciler, instance *v1alpha1.DatadogGenericCR) error {
	_, err := updateSyntheticsAPITest(r.datadogAuth, r.datadogSyntheticsClient, instance)
	return err
}
func (h *SyntheticsAPITestHandler) deleteResourcefunc(r *Reconciler, instance *v1alpha1.DatadogGenericCR) error {
	return deleteSyntheticTest(r.datadogAuth, r.datadogSyntheticsClient, instance.Status.Id)
}

// ************************************************************************************

type SyntheticsBrowserTestHandler struct{}

func (h *SyntheticsBrowserTestHandler) createResourcefunc(r *Reconciler, logger logr.Logger, instance *v1alpha1.DatadogGenericCR, status *v1alpha1.DatadogGenericCRStatus, now metav1.Time, hash string) error {
	createdTest, err := createSyntheticsAPITest(r.datadogAuth, r.datadogSyntheticsClient, instance)
	if err != nil {
		logger.Error(err, "error creating API test")
		updateErrStatus(status, now, v1alpha1.DatadogSyncStatusCreateError, "CreatingCustomResource", err)
		return err
	}
	additionalProperties := createdTest.AdditionalProperties
	return updateStatusFromSyntheticsTest(&createdTest, additionalProperties, status, logger, hash)
}

func (h *SyntheticsBrowserTestHandler) getResourcefunc(r *Reconciler, instance *v1alpha1.DatadogGenericCR) error {
	_, err := getSyntheticsTest(r.datadogAuth, r.datadogSyntheticsClient, instance.Status.Id)
	return err
}
func (h *SyntheticsBrowserTestHandler) updateResourcefunc(r *Reconciler, instance *v1alpha1.DatadogGenericCR) error {
	_, err := updateSyntheticsBrowserTest(r.datadogAuth, r.datadogSyntheticsClient, instance)
	return err
}
func (h *SyntheticsBrowserTestHandler) deleteResourcefunc(r *Reconciler, instance *v1alpha1.DatadogGenericCR) error {
	return deleteSyntheticTest(r.datadogAuth, r.datadogSyntheticsClient, instance.Status.Id)
}

// Synthetic tests (encompass browser and API tests): get
func getSyntheticsTest(auth context.Context, client *datadogV1.SyntheticsApi, testID string) (datadogV1.SyntheticsTestDetails, error) {
	test, _, err := client.GetTest(auth, testID)
	if err != nil {
		return datadogV1.SyntheticsTestDetails{}, translateClientError(err, "error getting synthetic test")
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
	if _, _, err := client.DeleteTests(auth, body); err != nil {
		return translateClientError(err, "error deleting synthetic test")
	}
	return nil
}

// Browser test: create
func createSyntheticBrowserTest(auth context.Context, client *datadogV1.SyntheticsApi, instance *v1alpha1.DatadogGenericCR) (datadogV1.SyntheticsBrowserTest, error) {
	browserTestBody := &datadogV1.SyntheticsBrowserTest{}
	json.Unmarshal([]byte(instance.Spec.JsonSpec), browserTestBody)
	test, _, err := client.CreateSyntheticsBrowserTest(auth, *browserTestBody)
	if err != nil {
		return datadogV1.SyntheticsBrowserTest{}, translateClientError(err, "error creating browser test")
	}
	return test, nil
}

// Browser test: update
func updateSyntheticsBrowserTest(auth context.Context, client *datadogV1.SyntheticsApi, instance *v1alpha1.DatadogGenericCR) (datadogV1.SyntheticsBrowserTest, error) {
	browserTestBody := &datadogV1.SyntheticsBrowserTest{}
	json.Unmarshal([]byte(instance.Spec.JsonSpec), browserTestBody)
	testUpdated, _, err := client.UpdateBrowserTest(auth, instance.Status.Id, *browserTestBody)
	if err != nil {
		return datadogV1.SyntheticsBrowserTest{}, translateClientError(err, "error updating browser test")
	}
	return testUpdated, nil
}

// API test: create
func createSyntheticsAPITest(auth context.Context, client *datadogV1.SyntheticsApi, instance *v1alpha1.DatadogGenericCR) (datadogV1.SyntheticsAPITest, error) {
	apiTestBody := &datadogV1.SyntheticsAPITest{}
	json.Unmarshal([]byte(instance.Spec.JsonSpec), apiTestBody)
	test, _, err := client.CreateSyntheticsAPITest(auth, *apiTestBody)
	if err != nil {
		return datadogV1.SyntheticsAPITest{}, translateClientError(err, "error creating API test")
	}
	return test, nil
}

// API test: update
func updateSyntheticsAPITest(auth context.Context, client *datadogV1.SyntheticsApi, instance *v1alpha1.DatadogGenericCR) (datadogV1.SyntheticsAPITest, error) {
	apiTestBody := &datadogV1.SyntheticsAPITest{}
	json.Unmarshal([]byte(instance.Spec.JsonSpec), apiTestBody)
	testUpdated, _, err := client.UpdateAPITest(auth, instance.Status.Id, *apiTestBody)
	if err != nil {
		return datadogV1.SyntheticsAPITest{}, translateClientError(err, "error updating API test")
	}
	return testUpdated, nil
}
