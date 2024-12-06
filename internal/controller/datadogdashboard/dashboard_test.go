package datadogdashboard

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	datadogapi "github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
	v1alpha1 "github.com/DataDog/datadog-operator/api/crds/datadoghq/v1alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/crds/utils"
	"github.com/stretchr/testify/assert"
)

const dateFormat = "2006-01-02 15:04:05.999999999 -0700 MST"

func TestBuildDashboard(t *testing.T) {
	templateVariables := []v1alpha1.DashboardTemplateVariable{
		{
			AvailableValues: &[]string{
				"foo",
				"bar",
			},
			Name: "test variable",
		},
	}
	templateVariablePresets := []v1alpha1.DashboardTemplateVariablePreset{
		{
			Name: apiutils.NewStringPointer("test preset"),
			TemplateVariables: []v1alpha1.DashboardTemplateVariablePresetValue{
				{
					Name: apiutils.NewStringPointer("foo-bar"),
					Values: []string{
						"foo",
						"bar",
					},
				},
			},
		},
	}

	db := &v1alpha1.DatadogDashboard{
		Spec: v1alpha1.DatadogDashboardSpec{
			LayoutType: "ordered",
			NotifyList: []string{
				"foo",
				"bar",
			},
			ReflowType: datadogV1.DASHBOARDREFLOWTYPE_AUTO.Ptr(),
			Tags: []string{
				"team:test",
				"team:foo-bar",
			},
			TemplateVariablePresets: templateVariablePresets,
			TemplateVariables:       templateVariables,
			Title:                   "test dashboard",
			Widgets:                 "",
		},
	}

	dashboard := buildDashboard(testLogger, db)
	assert.Equal(t, datadogV1.DashboardLayoutType(db.Spec.LayoutType), dashboard.GetLayoutType(), "discrepancy found in parameter: Query")
	assert.Equal(t, db.Spec.NotifyList, dashboard.GetNotifyList(), "discrepancy found in parameter: NotifyList")
	assert.Equal(t, datadogV1.DashboardReflowType(*db.Spec.ReflowType), dashboard.GetReflowType(), "discrepancy found in parameter: ReflowType")
	assert.Equal(t, db.Spec.Tags, dashboard.GetTags(), "discrepancy found in parameter: Tags")
	assert.Equal(t, db.Spec.Title, dashboard.GetTitle(), "discrepancy found in parameter: Title")
}

func Test_getDashboard(t *testing.T) {
	dbID := "test_id"
	expectedDashboard := genericDashboard(dbID)

	jsonDashboard, _ := expectedDashboard.MarshalJSON()
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(jsonDashboard)
	}))
	defer httpServer.Close()

	testConfig := datadogapi.NewConfiguration()
	testConfig.HTTPClient = httpServer.Client()
	apiClient := datadogapi.NewAPIClient(testConfig)
	client := datadogV1.NewDashboardsApi(apiClient)
	testAuth := setupTestAuth(httpServer.URL)

	val, err := getDashboard(testAuth, client, dbID)
	assert.Nil(t, err)
	assert.Equal(t, expectedDashboard, val)
}

func Test_createDashboard(t *testing.T) {
	dbId := "test_id"
	expectedDashboard := genericDashboard(dbId)

	db := &v1alpha1.DatadogDashboard{
		Spec: v1alpha1.DatadogDashboardSpec{
			LayoutType: datadogV1.DASHBOARDLAYOUTTYPE_ORDERED,
			NotifyList: []string{
				"test@example.com",
				"test2@example.com"},
			Title: "Test dashboard",
			Tags: []string{
				"team:test", "team:test2",
			},
		},
	}

	jsonDashboard, _ := expectedDashboard.MarshalJSON()
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(jsonDashboard)
	}))
	defer httpServer.Close()

	testConfig := datadogapi.NewConfiguration()
	testConfig.HTTPClient = httpServer.Client()
	apiClient := datadogapi.NewAPIClient(testConfig)
	client := datadogV1.NewDashboardsApi(apiClient)
	testAuth := setupTestAuth(httpServer.URL)

	dashboard, err := createDashboard(testAuth, testLogger, client, db)
	assert.Nil(t, err)

	assert.Equal(t, datadogV1.DashboardLayoutType(db.Spec.LayoutType), dashboard.GetLayoutType(), "discrepancy found in parameter: LayoutType")
	assert.Equal(t, db.Spec.Title, dashboard.GetTitle(), "discrepancy found in parameter: Title")
	assert.Equal(t, db.Spec.Tags, dashboard.GetTags(), "discrepancy found in parameter: Tags")
}

func Test_deleteDashboard(t *testing.T) {
	dbId := "test_id"

	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
	}))
	defer httpServer.Close()

	testConfig := datadogapi.NewConfiguration()
	testConfig.HTTPClient = httpServer.Client()
	apiClient := datadogapi.NewAPIClient(testConfig)
	client := datadogV1.NewDashboardsApi(apiClient)
	testAuth := setupTestAuth(httpServer.URL)

	err := deleteDashboard(testAuth, client, dbId)
	assert.Nil(t, err)
}

func Test_updateDashboard(t *testing.T) {
	dbId := "test_id"
	expectedDashboard := genericDashboard(dbId)

	db := &v1alpha1.DatadogDashboard{
		Spec: v1alpha1.DatadogDashboardSpec{
			LayoutType: datadogV1.DASHBOARDLAYOUTTYPE_ORDERED,
			NotifyList: []string{
				"test@example.com",
				"test2@example.com"},
			Title: "Test dashboard",
			Tags: []string{
				"team:test", "team:test2",
			},
		},
	}

	jsonDashboard, _ := expectedDashboard.MarshalJSON()
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(jsonDashboard)
	}))
	defer httpServer.Close()

	testConfig := datadogapi.NewConfiguration()
	testConfig.HTTPClient = httpServer.Client()
	apiClient := datadogapi.NewAPIClient(testConfig)
	client := datadogV1.NewDashboardsApi(apiClient)
	testAuth := setupTestAuth(httpServer.URL)

	dashboard, err := updateDashboard(testAuth, testLogger, client, db)
	assert.Nil(t, err)

	assert.Equal(t, datadogV1.DashboardLayoutType(db.Spec.LayoutType), dashboard.GetLayoutType(), "discrepancy found in parameter: LayoutType")
	assert.Equal(t, db.Spec.Title, dashboard.GetTitle(), "discrepancy found in parameter: Title")
	assert.Equal(t, db.Spec.Tags, dashboard.GetTags(), "discrepancy found in parameter: Tags")
}

func genericDashboard(dbID string) datadogV1.Dashboard {
	fakeRawNow := time.Unix(1612244495, 0)
	fakeNow, _ := time.Parse(dateFormat, strings.Split(fakeRawNow.String(), " db=")[0])
	layoutType := datadogV1.DashboardLayoutType("ordered")
	reflowType := datadogV1.DashboardReflowType("auto")
	notifyList := datadogapi.NewNullableList(&[]string{
		"test@example.com",
		"test2@example.com",
	})
	title := "Test dashboard"
	handle := "test_user"
	description := datadogapi.NewNullableString(apiutils.NewStringPointer("test description"))
	tags := datadogapi.NewNullableList(&[]string{
		"team:test", "team:test2",
	})

	return datadogV1.Dashboard{
		AuthorHandle: &handle,
		CreatedAt:    &fakeNow,
		Description:  *description,
		Tags:         *tags,
		Id:           &dbID,
		Title:        title,
		LayoutType:   layoutType,
		NotifyList:   *notifyList,
		ReflowType:   &reflowType,
		Widgets:      []datadogV1.Widget{},
	}
}

// Test unmarshalling of widgets
func TestUnmarshalWidgets(t *testing.T) {
	widgetsJson := readFile("widgets.json")
	widgetList := &[]datadogV1.Widget{}
	err := json.Unmarshal(widgetsJson, widgetList)

	assert.Nil(t, err)
	assert.Equal(t, 1, len(*widgetList))
}

func readFile(path string) []byte {
	f, err := os.Open(path)
	if err != nil {
		panic(fmt.Sprintf("cannot open file %q: %s", path, err))
	}

	defer func() {
		if err = f.Close(); err != nil {
			panic(fmt.Sprintf("cannot close file: %s", err))
		}
	}()

	b, err := io.ReadAll(f)
	if err != nil {
		panic(fmt.Sprintf("cannot read file %q: %s", path, err))
	}

	return b
}

func setupTestAuth(apiURL string) context.Context {
	testAuth := context.WithValue(
		context.Background(),
		datadogapi.ContextAPIKeys,
		map[string]datadogapi.APIKey{
			"apiKeyAuth": {
				Key: "DUMMY_API_KEY",
			},
			"appKeyAuth": {
				Key: "DUMMY_APP_KEY",
			},
		},
	)
	parsedAPIURL, _ := url.Parse(apiURL)
	testAuth = context.WithValue(testAuth, datadogapi.ContextServerIndex, 1)
	testAuth = context.WithValue(testAuth, datadogapi.ContextServerVariables, map[string]string{
		"name":     parsedAPIURL.Host,
		"protocol": parsedAPIURL.Scheme,
	})

	return testAuth
}

func Test_translateClientError(t *testing.T) {
	var ErrGeneric = errors.New("generic error")

	testCases := []struct {
		name                   string
		error                  error
		message                string
		expectedErrorType      error
		expectedError          error
		expectedErrorInterface interface{}
	}{
		{
			name:              "no message, generic error",
			error:             ErrGeneric,
			message:           "",
			expectedErrorType: ErrGeneric,
		},
		{
			name:              "generic message, generic error",
			error:             ErrGeneric,
			message:           "generic message",
			expectedErrorType: ErrGeneric,
		},
		{
			name:                   "generic message, error type datadogV1.GenericOpenAPIError",
			error:                  datadogapi.GenericOpenAPIError{},
			message:                "generic message",
			expectedErrorInterface: &datadogapi.GenericOpenAPIError{},
		},
		{
			name:          "generic message, error type *url.Error",
			error:         &url.Error{Err: fmt.Errorf("generic url error")},
			message:       "generic message",
			expectedError: fmt.Errorf("generic message (url.Error):  \"\": generic url error"),
		},
	}
	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			result := translateClientError(test.error, test.message)

			if test.expectedErrorType != nil {
				assert.True(t, errors.Is(result, test.expectedErrorType))
			}

			if test.expectedErrorInterface != nil {
				assert.True(t, errors.As(result, test.expectedErrorInterface))
			}

			if test.expectedError != nil {
				assert.Equal(t, test.expectedError, result)
			}
		})
	}
}
