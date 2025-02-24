package fake

import (
	"testing"

	v1 "k8s.io/api/core/v1"

	"github.com/DataDog/datadog-operator/api/datadoghq/common"
	merger "github.com/DataDog/datadog-operator/internal/controller/datadogagent/merger"
)

// EnvVarManager is an autogenerated mock type for the EnvVarManager type
type EnvVarManager struct {
	EnvVarsByC map[common.AgentContainerName][]*v1.EnvVar

	t testing.TB
}

// AddEnvVar provides a mock function with given fields: newEnvVar
func (_m *EnvVarManager) AddEnvVar(newEnvVar *v1.EnvVar) {
	_m.t.Logf("AddEnvVar %s: %#v", newEnvVar.Name, newEnvVar.Value)
	_m.EnvVarsByC[AllContainers] = append(_m.EnvVarsByC[AllContainers], newEnvVar)
}

// AddEnvVarToContainer provides a mock function with given fields: containerName, newEnvVar
func (_m *EnvVarManager) AddEnvVarToContainer(containerName common.AgentContainerName, newEnvVar *v1.EnvVar) {
	isInitContainer := false
	for _, initContainerName := range initContainerNames {
		if containerName == initContainerName {
			isInitContainer = true
			break
		}
	}
	if !isInitContainer {
		_m.t.Logf("AddEnvVar %s: %#v", newEnvVar.Name, newEnvVar.Value)
		_m.EnvVarsByC[containerName] = append(_m.EnvVarsByC[containerName], newEnvVar)
	}
}

// AddEnvVarToContainers provides a mock function with given fields: containerNames, newEnvVar
func (_m *EnvVarManager) AddEnvVarToContainers(containerNames []common.AgentContainerName, newEnvVar *v1.EnvVar) {
	for _, containerName := range containerNames {
		_m.AddEnvVarToContainer(containerName, newEnvVar)
	}
}

// AddEnvVarToInitContainer provides a mock function with given fields: containerName, newEnvVar
func (_m *EnvVarManager) AddEnvVarToInitContainer(containerName common.AgentContainerName, newEnvVar *v1.EnvVar) {
	for _, initContainerName := range initContainerNames {
		if containerName == initContainerName {
			_m.t.Logf("AddEnvVar to container %s key:%s value:%#v", containerName, newEnvVar.Name, newEnvVar.Value)
			_m.EnvVarsByC[containerName] = append(_m.EnvVarsByC[containerName], newEnvVar)
		}
	}
}

// AddEnvVarToContainerWithMergeFunc provides a mock function with given fields: containerName, newEnvVar, mergeFunc
func (_m *EnvVarManager) AddEnvVarToContainerWithMergeFunc(containerName common.AgentContainerName, newEnvVar *v1.EnvVar, mergeFunc merger.EnvVarMergeFunction) error {
	found := false
	idFound := 0
	for id, envVar := range _m.EnvVarsByC[containerName] {
		if envVar.Name == newEnvVar.Name {
			found = true
			idFound = id
		}
	}

	if found {
		var err error
		newEnvVar, err = mergeFunc(_m.EnvVarsByC[containerName][idFound], newEnvVar)
		_m.EnvVarsByC[containerName][idFound] = newEnvVar
		return err
	}

	_m.EnvVarsByC[containerName] = append(_m.EnvVarsByC[containerName], newEnvVar)
	return nil
}

// AddEnvVarWithMergeFunc provides a mock function with given fields: newEnvVar, mergeFunc
func (_m *EnvVarManager) AddEnvVarWithMergeFunc(newEnvVar *v1.EnvVar, mergeFunc merger.EnvVarMergeFunction) error {
	return _m.AddEnvVarToContainerWithMergeFunc(AllContainers, newEnvVar, mergeFunc)
}

// NewFakeEnvVarManager creates a new instance of EnvVarManager. It also registers the testing.TB interface on the mock and a cleanup function to assert the mocks expectations.
func NewFakeEnvVarManager(t testing.TB) *EnvVarManager {
	return &EnvVarManager{
		EnvVarsByC: make(map[common.AgentContainerName][]*v1.EnvVar),
		t:          t,
	}
}
