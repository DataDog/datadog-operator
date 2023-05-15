// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package state

import (
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/secure-systems-lab/go-securesystemslib/cjson"
	"github.com/stretchr/testify/assert"
)

func TestNewRepositoryWithNilRoot(t *testing.T) {
	repository, err := NewRepository(nil)
	assert.Nil(t, repository, "Creating a repository without a starting base root should result in an error per TUF spec")
	assert.Error(t, err, "Creating a repository without a starting base root should result in an error per TUF spec")
}

func TestNewRepositoryWithMalformedRoot(t *testing.T) {
	repository, err := NewRepository([]byte("haha I am not a real root"))
	assert.Nil(t, repository, "Creating a repository with a malformed base root should result in an error per TUF spec")
	assert.Error(t, err, "Creating a repository with a malformed base root should result in an error per TUF spec")
}

func TestNewUnverifiedRepository(t *testing.T) {
	repository, err := NewUnverifiedRepository()
	assert.NotNil(t, repository, "Creating an unverified repository should always succeed")
	assert.Nil(t, err, "Creating an unverified repository should always succeed with no error")
}

// TestEmptyUpdateZeroTypes makes sure that a properly initialized, but otherwise empty,
// `Update` won't cause an error, crash the Update process, and also results in unchanged state.
func TestEmptyUpdateZeroTypes(t *testing.T) {
	ta := newTestArtifacts()

	emptyUpdate := Update{
		TUFRoots:      make([][]byte, 0),
		TUFTargets:    make([]byte, 0),
		TargetFiles:   make(map[string][]byte),
		ClientConfigs: make([]string, 0),
	}

	r := ta.repository

	updatedProducts, err := r.Update(emptyUpdate)
	assert.Nil(t, err)
	assert.Equal(t, 0, len(updatedProducts), "An empty update shouldn't indicate any updated products")

	state, err := r.CurrentState()
	assert.NoError(t, err)
	assert.Equal(t, 0, len(state.Configs))
	assert.Equal(t, 0, len(state.CachedFiles))
	assert.EqualValues(t, 0, state.TargetsVersion)
	assert.EqualValues(t, 1, state.RootsVersion)
	assert.Nil(t, state.OpaqueBackendState)

	// Do the same with the unverified repository, there should be no functional difference.
	r = ta.unverifiedRepository

	updatedProducts, err = r.Update(emptyUpdate)
	assert.Nil(t, err)
	assert.Equal(t, 0, len(updatedProducts), "An empty update shouldn't indicate any updated products")

	state, err = r.CurrentState()
	assert.NoError(t, err)
	assert.Equal(t, 0, len(state.Configs))
	assert.Equal(t, 0, len(state.CachedFiles))
	assert.EqualValues(t, 0, state.TargetsVersion)
	assert.EqualValues(t, 1, state.RootsVersion)
	assert.Nil(t, state.OpaqueBackendState)
}

// TestEmptyUpdateNilTypes makes sure that a completely uninitialized `Update` field won't
// cause an error, crash the Update process, and also results in unchanged state.
func TestEmptyUpdateNilTypes(t *testing.T) {
	ta := newTestArtifacts()

	emptyUpdate := Update{
		TUFRoots:      nil,
		TUFTargets:    nil,
		TargetFiles:   nil,
		ClientConfigs: nil,
	}

	r := ta.repository

	updatedProducts, err := r.Update(emptyUpdate)
	assert.Nil(t, err)
	assert.Equal(t, 0, len(updatedProducts), "An empty update shouldn't indicate any updated products")

	state, err := r.CurrentState()
	assert.NoError(t, err)
	assert.Equal(t, 0, len(state.Configs))
	assert.Equal(t, 0, len(state.CachedFiles))
	assert.EqualValues(t, 0, state.TargetsVersion)
	assert.EqualValues(t, 1, state.RootsVersion)
	assert.Nil(t, state.OpaqueBackendState)

	// Do the same with the unverified repository, there should be no functional difference.
	r = ta.unverifiedRepository

	updatedProducts, err = r.Update(emptyUpdate)
	assert.Nil(t, err)
	assert.Equal(t, 0, len(updatedProducts), "An empty update shouldn't indicate any updated products")

	state, err = r.CurrentState()
	assert.NoError(t, err)
	assert.Equal(t, 0, len(state.Configs))
	assert.Equal(t, 0, len(state.CachedFiles))
	assert.EqualValues(t, 0, state.TargetsVersion)
	assert.EqualValues(t, 1, state.RootsVersion)
	assert.Nil(t, state.OpaqueBackendState)
}

func createRawTestData(targetFileBase string, targetsPayload string) []byte {
	payload := base64.StdEncoding.EncodeToString([]byte(targetsPayload))
	targets := make(map[string]interface{})
	if err := json.Unmarshal([]byte(targetFileBase), &targets); err != nil {
		panic(err)
	}

	targets["targets"] = payload
	marshalled, err := cjson.EncodeCanonical(targets)
	if err != nil {
		panic(err)
	}
	return marshalled
}

// These tests involve generated JSON responses that the remote config service would send. They
// were primarily created to assist tracer teams with unit tests around TUF integrity checks,
// but they apply to agent clients as well, so we include them here as an extra layer of protection.
//
// These should only be done against the verified version of the repository.
func TestPreGeneratedIntegrityChecks(t *testing.T) {
	testRoot := []byte(`{"signed":{"_type":"root","spec_version":"1.0","version":1,"expires":"2032-05-29T12:49:41.030418-04:00","keys":{"ed7672c9a24abda78872ee32ee71c7cb1d5235e8db4ecbf1ca28b9c50eb75d9e":{"keytype":"ed25519","scheme":"ed25519","keyid_hash_algorithms":["sha256","sha512"],"keyval":{"public":"7d3102e39abe71044d207550bda239c71380d013ec5a115f79f51622630054e6"}}},"roles":{"root":{"keyids":["ed7672c9a24abda78872ee32ee71c7cb1d5235e8db4ecbf1ca28b9c50eb75d9e"],"threshold":1},"snapshot":{"keyids":["ed7672c9a24abda78872ee32ee71c7cb1d5235e8db4ecbf1ca28b9c50eb75d9e"],"threshold":1},"targets":{"keyids":["ed7672c9a24abda78872ee32ee71c7cb1d5235e8db4ecbf1ca28b9c50eb75d9e"],"threshold":1},"timestsmp":{"keyids":["ed7672c9a24abda78872ee32ee71c7cb1d5235e8db4ecbf1ca28b9c50eb75d9e"],"threshold":1}},"consistent_snapshot":true},"signatures":[{"keyid":"ed7672c9a24abda78872ee32ee71c7cb1d5235e8db4ecbf1ca28b9c50eb75d9e","sig":"d7e24828d1d3104e48911860a13dd6ad3f4f96d45a9ea28c4a0f04dbd3ca6c205ed406523c6c4cacfb7ebba68f7e122e42746d1c1a83ffa89c8bccb6f7af5e06"}]}`)

	type testData struct {
		description string
		isError     bool
		rawUpdate   []byte
	}

	type pregeneratedResponse struct {
		Targets     []byte `json:"targets"`
		TargetFiles []struct {
			Path string `json:"path"`
			Raw  []byte `json:"raw"`
		} `json:"target_files"`
		ClientConfigs []string `json:"client_configs"`
	}

	tests := []testData{

		{description: "valid", isError: false, rawUpdate: createRawTestData(
			`{"target_files":[{"path":"datadog/2/ASM_FEATURES/ASM_FEATURES-base/config","raw":"ewogICAgImFzbSI6IHsKICAgICAgICAiZW5hYmxlZCI6IHRydWUKICAgIH0KfQo="}],"client_configs":["datadog/2/ASM_FEATURES/ASM_FEATURES-base/config"]}`,
			`{"signed":{"_type":"targets","custom":{"opaque_backend_state":"eyJmb28iOiAiYmFyIn0="},"expires":"2032-10-24T15:10:45.097315-04:00","spec_version":"1.0","targets":{"datadog/2/ASM_FEATURES/ASM_FEATURES-base/config":{"custom":{"v":1},"hashes":{"sha256":"9221dfd9f6084151313e3e4920121ae843614c328e4630ea371ba66e2f15a0a6"},"length":47}},"version":1},"signatures":[{"keyid":"ed7672c9a24abda78872ee32ee71c7cb1d5235e8db4ecbf1ca28b9c50eb75d9e","sig":"8cf4603262262fb06146868ccf46092e120a82ce5c45fbfd8dd52aae807fe3d0450cac8459c32cd2848951a0482341b04818e4bc062d0614f05621db950b3c0b"}]}`),
		},

		{description: "invalid tuf targets signature", isError: true, rawUpdate: createRawTestData(
			`{"target_files":[{"path":"datadog/2/ASM_FEATURES/ASM_FEATURES-base/config","raw":"ewogICAgImFzbSI6IHsKICAgICAgICAiZW5hYmxlZCI6IHRydWUKICAgIH0KfQo="}],"client_configs":["datadog/2/ASM_FEATURES/ASM_FEATURES-base/config"]}`,
			`{"signed":{"_type":"targets","spec_version":"1.0","version":999,"expires":"2032-10-24T15:10:45.097315-04:00","targets":{"datadog/2/ASM_FEATURES/ASM_FEATURES-base/config":{"length":47,"hashes":{"sha256":"9221dfd9f6084151313e3e4920121ae843614c328e4630ea371ba66e2f15a0a6"},"custom":{"v":1}}},"custom":{"opaque_backend_state":"eyJmb28iOiAiYmFyIn0="}},"signatures":[{"keyid":"ed7672c9a24abda78872ee32ee71c7cb1d5235e8db4ecbf1ca28b9c50eb75d9e","sig":"8cf4603262262fb06146868ccf46092e120a82ce5c45fbfd8dd52aae807fe3d0450cac8459c32cd2848951a0482341b04818e4bc062d0614f05621db950b3c0b"}]}`),
		},

		{description: "tuf targets signed with invalid key", isError: true, rawUpdate: createRawTestData(
			`{"target_files":[{"path":"datadog/2/ASM_FEATURES/ASM_FEATURES-base/config","raw":"ewogICAgImFzbSI6IHsKICAgICAgICAiZW5hYmxlZCI6IHRydWUKICAgIH0KfQo="}],"client_configs":["datadog/2/ASM_FEATURES/ASM_FEATURES-base/config"]}`,
			`{"signed":{"_type":"targets","custom":{"opaque_backend_state":"eyJmb28iOiAiYmFyIn0="},"expires":"2032-10-24T15:10:45.097315-04:00","spec_version":"1.0","targets":{},"version":1},"signatures":[{"keyid":"ed7672c9a24abda78872ee32ee71c7cb1d5235e8db4ecbf1ca28b9c50eb75d9e","sig":"ef3cfcf821f6c191232f2b0af5931a15ac91d363c781d1195feaf11f1b50a2343c708e2c9bfe64d56415240c12a9afcbd2f556d3a9f479e94efc4516d5934907"}]}`),
		},

		{description: "missing target file in tuf targets", isError: true, rawUpdate: createRawTestData(
			`{"target_files":[{"path":"datadog/2/ASM_FEATURES/ASM_FEATURES-base/config","raw":"ewogICAgImFzbSI6IHsKICAgICAgICAiZW5hYmxlZCI6IHRydWUKICAgIH0KfQo="}],"client_configs":["datadog/2/ASM_FEATURES/ASM_FEATURES-base/config"]}`,
			`{"signed":{"_type":"targets","custom":{"opaque_backend_state":"eyJmb28iOiAiYmFyIn0="},"expires":"2032-10-24T15:10:45.097315-04:00","spec_version":"1.0","targets":{},"version":1},"signatures":[{"keyid":"ed7672c9a24abda78872ee32ee71c7cb1d5235e8db4ecbf1ca28b9c50eb75d9e","sig":"ef3cfcf821f6c191232f2b0af5931a15ac91d363c781d1195feaf11f1b50a2343c708e2c9bfe64d56415240c12a9afcbd2f556d3a9f479e94efc4516d5934907"}]}`,
		)},
		{description: "target file hash incorrect in tuf targets", isError: true, rawUpdate: createRawTestData(
			`{"target_files":[{"path":"datadog/2/ASM_FEATURES/ASM_FEATURES-base/config","raw":"ewogICAgImFzbSI6IHsKICAgICAgICAiZW5hYmxlZCI6IHRydWUKICAgIH0KfQo="}],"client_configs":["datadog/2/ASM_FEATURES/ASM_FEATURES-base/config"]}`,
			`{"signed":{"_type":"targets","custom":{"opaque_backend_state":"eyJmb28iOiAiYmFyIn0="},"expires":"2032-10-24T15:10:45.097315-04:00","spec_version":"1.0","targets":{"datadog/2/ASM_FEATURES/ASM_FEATURES-base/config":{"custom":{"v":1},"hashes":{"sha256":"66616b6568617368"},"length":47}},"version":1},"signatures":[{"keyid":"ed7672c9a24abda78872ee32ee71c7cb1d5235e8db4ecbf1ca28b9c50eb75d9e","sig":"a323d0f5535b3a309273b5a3df70f36bfa3e0e74acf98307148dc41f6cc02982654b48c7264e64ab2649acde732f5808ec74f37feba6410310b0a5d40dc5250c"}]}`,
		)},
		{description: "target file length incorrect in tuf targets", isError: true, rawUpdate: createRawTestData(
			`{"target_files":[{"path":"datadog/2/ASM_FEATURES/ASM_FEATURES-base/config","raw":"ewogICAgImFzbSI6IHsKICAgICAgICAiZW5hYmxlZCI6IHRydWUKICAgIH0KfQo="}],"client_configs":["datadog/2/ASM_FEATURES/ASM_FEATURES-base/config"]}`,
			`{"signed":{"_type":"targets","custom":{"opaque_backend_state":"eyJmb28iOiAiYmFyIn0="},"expires":"2032-10-24T15:10:45.097315-04:00","spec_version":"1.0","targets":{"datadog/2/ASM_FEATURES/ASM_FEATURES-base/config":{"custom":{"v":1},"hashes":{"sha256":"66616b6568617368"},"length":999}},"version":1},"signatures":[{"keyid":"ed7672c9a24abda78872ee32ee71c7cb1d5235e8db4ecbf1ca28b9c50eb75d9e","sig":"664dd2dfc841717a66101cddff1123f142bed3200790724a3b16eb016ef7944a139f61ce9a3833a525b823ae093f9bb986aae84d23feb1d855644d265b349d0e"}]}`,
		)},
	}

	for _, test := range tests {
		// These payloads are the ClientGetConfigsResponse JSON from the protobuf layer, so we have
		// to do a little processing first to be able to use them here in this internal package that is protobuf layer agnostic.
		var parsed pregeneratedResponse
		err := json.Unmarshal(test.rawUpdate, &parsed)
		assert.NoError(t, err)
		updateFiles := make(map[string][]byte)
		for _, f := range parsed.TargetFiles {
			updateFiles[f.Path] = f.Raw
		}
		update := Update{
			TUFTargets:    parsed.Targets,
			TargetFiles:   updateFiles,
			ClientConfigs: parsed.ClientConfigs,
		}

		repository, err := NewRepository(testRoot)
		assert.NoError(t, err)

		result, err := repository.Update(update)
		if test.isError {
			assert.Error(t, err, test.description)
			assert.Nil(t, result)
		} else {
			assert.NoError(t, err)
			assert.NotNil(t, result)
		}
	}
}
