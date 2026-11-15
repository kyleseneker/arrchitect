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
		want   []int
	}{
		{name: "none declared", want: nil},
		{name: "an empty list is no tags", labels: []string{}, want: []int{}},
		{name: "labels resolve once each", labels: []string{"anime", "4k", "anime"}, want: []int{5, 3}},
		{name: "labels match as the apps store them, in lowercase", labels: []string{"4K", "Anime"}, want: []int{3, 5}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ids.ResolveTags(tt.labels)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}

	_, err := ids.ResolveTags([]string{"missing"})
	assert.ErrorContains(t, err, "missing")
}
