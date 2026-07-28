package feature

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestApplyProfileSharedConfigOverlaysNilTarget(t *testing.T) {
	err := ApplyProfileSharedConfigOverlays(nil, nil, nil)

	assert.ErrorContains(t, err, "profile shared config overlay target spec is nil")
}
