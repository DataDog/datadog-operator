package datadogdashboard

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"testing"

	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	datadogapi "github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
	v1alpha1 "github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
)

const dateFormat = "2006-01-02 15:04:05.999999999 -0700 MST"

var (
	testLogger logr.Logger = zap.New(zap.UseDevMode(true))
)

// Test a dashboard manifest with the data field set
func TestUnmarshallDashboard(t *testing.T) {

	dashboardJson := readFile("dashboard.json")
	v1alpha1Dashboard := v1alpha1.DatadogDashboard{}
	err := json.Unmarshal(dashboardJson, &v1alpha1Dashboard)
	dashboard := buildDashboard(testLogger, &v1alpha1Dashboard)

	templateVariables := []datadogV1.DashboardTemplateVariable{{
		Name:            "second",
		Prefix:          *datadogapi.NewNullableString(apiutils.NewStringPointer("override prefix")),
		AvailableValues: *datadogapi.NewNullableList(&[]string{"host1"}),
		Defaults:        []string{"*"},
	}}

	assert.Equal(t, nil, err)
	// Sanity check. Check that dashboard has the number of widgets we expect it to
	assert.Equal(t, 1, len(dashboard.GetWidgets()))
	// Check overriden fields
	assert.Equal(t, "Changed Title", dashboard.GetTitle())
	assert.Equal(t, []string{"test@datadoghq.com"}, dashboard.GetNotifyList())
	assert.Equal(t, templateVariables, dashboard.GetTemplateVariables())
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
