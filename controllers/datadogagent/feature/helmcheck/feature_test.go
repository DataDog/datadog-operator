// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package helmcheck

import (
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"

	apicommon "github.com/DataDog/datadog-operator/apis/datadoghq/common"
	apicommonv1 "github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
	v2alpha1test "github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1/test"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature/fake"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature/test"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/comparison"
)

var customConfData = `cluster_check: true
init_config:
instances:
  - collectEvents: true
    helm_values_as_tags:
      foo: bar
`

func Test_helmCheckFeature_Configure(t *testing.T) {
	tests := test.FeatureTestSuite{
		{
			Name: "Helm check disabled",
			DDAv2: v2alpha1test.NewDatadogAgentBuilder().
				WithHelmCheckEnabled(false).
				Build(),
			WantConfigure: false,
		},
		{
			Name: "Helm check enabled",
			DDAv2: v2alpha1test.NewDatadogAgentBuilder().
				WithHelmCheckEnabled(true).
				WithHelmCheckCollectEvents(true).
				WithHelmCheckValuesAsTags(map[string]string{"foo": "bar"}).
				WithHelmCheckCustomConfigData(customConfData).
				Build(),
			WantConfigure: true,
			ClusterAgent:  helmCheckClusterAgentWantFunc(),
		},
		{
			Name: "Helm check enabled and runs on cluster checks runner",
			DDAv2: v2alpha1test.NewDatadogAgentBuilder().
				WithHelmCheckEnabled(true).
				WithHelmCheckCollectEvents(true).
				WithHelmCheckValuesAsTags(map[string]string{"foo": "bar"}).
				WithHelmCheckCustomConfigData(customConfData).
				WithClusterChecksEnabled(true).
				WithClusterChecksUseCLCEnabled(true).
				Build(),
			WantConfigure: true,
			ClusterAgent:  helmCheckClusterAgentWantFunc(),
		},
	}

	tests.Run(t, buildHelmCheckFeature)
}

func helmCheckClusterAgentWantFunc() *test.ComponentTest {
	return test.NewDefaultComponentTest().WithWantFunc(
		func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
			mgr := mgrInterface.(*fake.PodTemplateManagers)

			customConfig := apicommonv1.CustomConfig{
				ConfigData: apiutils.NewStringPointer(customConfData),
			}
			hash, err := comparison.GenerateMD5ForSpec(&customConfig)
			assert.NoError(t, err)
			wantAnnotations := map[string]string{
				fmt.Sprintf(apicommon.MD5ChecksumAnnotationKey, feature.HelmCheckIDType): hash,
			}
			annotations := mgr.AnnotationMgr.Annotations
			assert.True(t, apiutils.IsEqualStruct(annotations, wantAnnotations), "Annotations \ndiff = %s", cmp.Diff(annotations, wantAnnotations))

		},
	)
}
