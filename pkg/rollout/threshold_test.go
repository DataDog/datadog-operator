// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package rollout

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseMaxUnavailable(t *testing.T) {
	for in, want := range map[string]MaxUnavailable{
		"0":    {},
		"3":    {Value: 3},
		" 2 ":  {Value: 2},
		"0%":   {Percent: true},
		"1%":   {Value: 1, Percent: true},
		"100%": {Value: 100, Percent: true},
	} {
		got, err := ParseMaxUnavailable(in)
		require.NoError(t, err, in)
		assert.Equal(t, want, got, in)
	}
	for _, in := range []string{"", "-1", "-1%", "101%", "1.5", "1.5%", "%", "abc", "5%%", "99999999999"} {
		_, err := ParseMaxUnavailable(in)
		assert.Error(t, err, in)
	}
}

func TestMaxUnavailableString(t *testing.T) {
	assert.Equal(t, "0", MaxUnavailable{}.String())
	assert.Equal(t, "2", MaxUnavailable{Value: 2}.String())
	assert.Equal(t, "10%", MaxUnavailable{Value: 10, Percent: true}.String())
}

func TestMaxUnavailableResolve(t *testing.T) {
	tests := []struct {
		m       MaxUnavailable
		desired int32
		want    int32
	}{
		{MaxUnavailable{}, 807, 0},
		{MaxUnavailable{Value: 1}, 2, 1},
		{MaxUnavailable{Value: 3}, 0, 3},
		{MaxUnavailable{Value: 1, Percent: true}, 2, 0},
		{MaxUnavailable{Value: 1, Percent: true}, 807, 8},
		{MaxUnavailable{Value: 50, Percent: true}, 2, 1},
		{MaxUnavailable{Value: 50, Percent: true}, 3, 1},
		{MaxUnavailable{Value: 100, Percent: true}, 5, 5},
		{MaxUnavailable{Value: 10, Percent: true}, 0, 0},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, tt.m.Resolve(tt.desired), "%s of %d", tt.m, tt.desired)
	}
}
