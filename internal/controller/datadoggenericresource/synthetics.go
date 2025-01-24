package datadoggenericresource

import (
	"context"
	"errors"
	"time"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
)

type SyntheticsCRUDClient struct {
	client *datadogV1.SyntheticsApi
}

func (c *SyntheticsCRUDClient) createResource(auth context.Context, unmarshaledSpec any) (any, error) {
	var test any
	var err error
	// We lose the `type` information when unmarshaling, so we need to switch on the type of the unmarshaled object
	// to determine which type of synthetic test we are creating
	// Else, we could in the unmarshaling step, add a field to the struct that would indicate the type of synthetic test
	switch v := unmarshaledSpec.(type) {
	case *datadogV1.SyntheticsAPITest:
		test, _, err = c.client.CreateSyntheticsAPITest(auth, *v)
	case *datadogV1.SyntheticsBrowserTest:
		test, _, err = c.client.CreateSyntheticsBrowserTest(auth, *v)
	default:
		return nil, errors.New("unknown synthetic test type")
	}
	if err != nil {
		return nil, err
	}
	return test, nil
}

func (c *SyntheticsCRUDClient) getResource(auth context.Context, resourceStringID string) error {
	_, _, err := c.client.GetTest(auth, resourceStringID)
	if err != nil {
		return translateClientError(err, "error getting synthetic test")
	}
	return nil
}

func (c *SyntheticsCRUDClient) updateResource(auth context.Context, resourceStringID string, unmarshaledSpec any) (any, error) {
	var test any
	var err error
	switch v := unmarshaledSpec.(type) {
	case *datadogV1.SyntheticsAPITest:
		test, _, err = c.client.UpdateAPITest(auth, resourceStringID, *v)
	case *datadogV1.SyntheticsBrowserTest:
		test, _, err = c.client.UpdateBrowserTest(auth, resourceStringID, *v)
	default:
		return nil, errors.New("unknown synthetic test type")
	}
	if err != nil {
		return nil, err
	}
	return test, nil
}

func (c *SyntheticsCRUDClient) deleteResource(auth context.Context, resourceStringID string) error {
	body := datadogV1.SyntheticsDeleteTestsPayload{
		PublicIds: []string{
			resourceStringID,
		},
	}
	if _, _, err := c.client.DeleteTests(auth, body); err != nil {
		return translateClientError(err, "error deleting synthetic test")
	}
	return nil
}

type SyntheticsAPITestHandler struct{}

func (h *SyntheticsAPITestHandler) createResourcefunc(r *Reconciler, logger logr.Logger, instance *v1alpha1.DatadogGenericResource, status *v1alpha1.DatadogGenericResourceStatus, now metav1.Time, hash string) error {
	resource, err := CreateResource(r.datadogAuth, &SyntheticsCRUDClient{client: r.datadogSyntheticsClient}, instance)
	if err != nil {
		logger.Error(err, "error creating API test")
		updateErrStatus(status, now, v1alpha1.DatadogSyncStatusCreateError, "CreatingCustomResource", err)
		return err
	}
	createdTest := resource.(datadogV1.SyntheticsAPITest)
	additionalProperties := createdTest.AdditionalProperties
	return updateStatusFromSyntheticsTest(&createdTest, additionalProperties, status, logger, hash)
}

func (h *SyntheticsAPITestHandler) getResourcefunc(r *Reconciler, instance *v1alpha1.DatadogGenericResource) error {
	return GetResource(r.datadogAuth, &SyntheticsCRUDClient{client: r.datadogSyntheticsClient}, instance)
}
func (h *SyntheticsAPITestHandler) updateResourcefunc(r *Reconciler, instance *v1alpha1.DatadogGenericResource) error {
	_, err := UpdateResource(r.datadogAuth, &SyntheticsCRUDClient{client: r.datadogSyntheticsClient}, instance)
	return err
}
func (h *SyntheticsAPITestHandler) deleteResourcefunc(r *Reconciler, instance *v1alpha1.DatadogGenericResource) error {
	return DeleteResource(r.datadogAuth, &SyntheticsCRUDClient{client: r.datadogSyntheticsClient}, instance)
}

type SyntheticsBrowserTestHandler struct{}

func (h *SyntheticsBrowserTestHandler) createResourcefunc(r *Reconciler, logger logr.Logger, instance *v1alpha1.DatadogGenericResource, status *v1alpha1.DatadogGenericResourceStatus, now metav1.Time, hash string) error {
	resource, err := CreateResource(r.datadogAuth, &SyntheticsCRUDClient{client: r.datadogSyntheticsClient}, instance)
	if err != nil {
		logger.Error(err, "error creating browser test")
		updateErrStatus(status, now, v1alpha1.DatadogSyncStatusCreateError, "CreatingCustomResource", err)
		return err
	}
	createdTest := resource.(datadogV1.SyntheticsBrowserTest)
	additionalProperties := createdTest.AdditionalProperties
	return updateStatusFromSyntheticsTest(&createdTest, additionalProperties, status, logger, hash)
}

func (h *SyntheticsBrowserTestHandler) getResourcefunc(r *Reconciler, instance *v1alpha1.DatadogGenericResource) error {
	return GetResource(r.datadogAuth, &SyntheticsCRUDClient{client: r.datadogSyntheticsClient}, instance)
}
func (h *SyntheticsBrowserTestHandler) updateResourcefunc(r *Reconciler, instance *v1alpha1.DatadogGenericResource) error {
	_, err := UpdateResource(r.datadogAuth, &SyntheticsCRUDClient{client: r.datadogSyntheticsClient}, instance)
	return err
}
func (h *SyntheticsBrowserTestHandler) deleteResourcefunc(r *Reconciler, instance *v1alpha1.DatadogGenericResource) error {
	return DeleteResource(r.datadogAuth, &SyntheticsCRUDClient{client: r.datadogSyntheticsClient}, instance)
}

// updateStatusFromSyntheticsTest retrieves the common fields from a synthetic test (API, browser) and updates the status of the DatadogGenericResource
func updateStatusFromSyntheticsTest(createdTest interface{ GetPublicId() string }, additionalProperties map[string]interface{}, status *v1alpha1.DatadogGenericResourceStatus, logger logr.Logger, hash string) error {
	// All synthetic test types share this method
	status.Id = createdTest.GetPublicId()

	// Parse Created Time
	createdTimeString, ok := additionalProperties["created_at"].(string)
	if !ok {
		logger.Error(nil, "missing or invalid created_at field, using current time")
		createdTimeString = time.Now().Format(time.RFC3339)
	}

	createdTimeParsed, err := time.Parse(time.RFC3339, createdTimeString)
	if err != nil {
		logger.Error(err, "error parsing created time, using current time")
		createdTimeParsed = time.Now()
	}
	createdTime := metav1.NewTime(createdTimeParsed)

	// Update status fields
	status.Created = &createdTime
	status.LastForceSyncTime = &createdTime

	// Update Creator
	if createdBy, ok := additionalProperties["created_by"].(map[string]interface{}); ok {
		if handle, ok := createdBy["handle"].(string); ok {
			status.Creator = handle
		} else {
			logger.Error(nil, "missing handle field in created_by")
			status.Creator = ""
		}
	} else {
		logger.Error(nil, "missing or invalid created_by field")
		status.Creator = ""
	}

	// Update Sync Status and Hash
	status.SyncStatus = v1alpha1.DatadogSyncStatusOK
	status.CurrentHash = hash

	return nil
}
