package engine

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveTags(t *testing.T) {
	ids := NamedIDs{"4k": {3}, "anime": {5}}
	tests := []struct {
		name   string
		labels []string
		legacy []int
		want   []int
	}{
		{name: "neither declared", want: nil},
		{name: "legacy IDs", legacy: []int{9}, want: []int{9}},
		{name: "an empty label list is no tags", labels: []string{}, want: []int{}},
		{name: "legacy IDs win over an empty label list", labels: []string{}, legacy: []int{9}, want: []int{9}},
		{name: "labels win over legacy IDs", labels: []string{"anime", "4k", "anime"}, legacy: []int{9}, want: []int{5, 3}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ids.ResolveTags(tt.labels, tt.legacy)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}

	_, err := ids.ResolveTags([]string{"missing"}, nil)
	assert.ErrorContains(t, err, "missing")
}
