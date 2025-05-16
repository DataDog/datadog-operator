package fake

import (
	"testing"
)

// AnnotationManager is a mock type for the AnnotationManager type
type AnnotationManager struct {
	Annotations map[string]string

	t testing.TB
}

// AddAnnotation provides a mock function with given fields: key, value
func (_m *AnnotationManager) AddAnnotation(key, value string) {
	_m.Annotations[key] = value
}

// NewFakeAnnotationManager creates a new instance of AnnotationManager. It also registers the testing.TB interface on the mock and a cleanup function to assert the mocks expectations.
func NewFakeAnnotationManager(t testing.TB) *AnnotationManager {
	return &AnnotationManager{
		Annotations: make(map[string]string),
		t:           t,
	}
}
