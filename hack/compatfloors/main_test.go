package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestAppVersion(t *testing.T) {
	for tag, want := range map[string]string{
		"4.0.20.3014-ls2":           "4.0.20",
		"v1.6.2-ls371":              "1.6.2",
		"develop-0.4.18.2805-ls157": "0.4.18",
		"5.3.1_v2.1.3-ls490":        "5.3.1",
		"release-5.3.1":             "5.3.1",
		"1.43.5.11079-b3b13e506":    "1.43.5",
		"12.4":                      "12.4",
		"3.08.00":                   "3.08.00",
		"latest":                    "",
	} {
		assert.Equal(t, want, appVersion(tag), tag)
	}
}

func TestShapeMatchesTagsInTheSameForm(t *testing.T) {
	for tag, cases := range map[string]map[string]bool{
		"4.0.20.3014-ls2": {
			"4.0.16.2944-ls304": true, "amd64-4.0.20.3014-ls2": false, "4.0.20.3014": false, "develop-4.0.20-ls1": false,
		},
		"5.3.1_v2.1.3-ls490": {
			"5.2.0_v2.0.11-ls470": true, "5.2.0_v1.2.20-ls470": false, "5.1.4-r3-ls453": false,
		},
		"1.43.5.11079-b3b13e506": {
			"1.42.2.10156-f737b826c": true, "1.42.2.10156-f737b826c-amd64": false, "beta": false,
		},
		"12.4":                      {"10.11.3": true, "10.11.3-amd64": false},
		"v1.91.1":                   {"v1.69.0": true, "1.69.0": false},
		"develop-0.4.18.2805-ls157": {"develop-0.4.17.2801-ls156": true, "0.4.17.2801-ls156": false},
	} {
		form := shape(tag)
		for candidate, want := range cases {
			assert.Equal(t, want, form.MatchString(candidate), "%s against the form of %s", candidate, tag)
		}
	}
}

func TestCompareVersions(t *testing.T) {
	assert.Equal(t, 1, compareVersions("3.10.00", "3.08.00"))
	assert.Equal(t, -1, compareVersions("10.11.3", "12.4"))
	assert.Equal(t, 0, compareVersions("3.5", "3.5.0"))
	assert.Equal(t, 1, compareVersions("4.0.21", "4.0.20"))
}

func TestFloorIsTheNewestReleaseOutByTheCutoff(t *testing.T) {
	cutoff := time.Date(2025, 11, 28, 0, 0, 0, 0, time.UTC)
	day := func(month time.Month, d int) time.Time { return time.Date(2025, month, d, 0, 0, 0, 0, time.UTC) }
	releases := []release{
		{version: "10.11.0", date: day(10, 20)},
		{version: "10.11.4", date: day(12, 15)},
		{version: "10.11.3", date: day(11, 18)},
		// A backport published later than a newer version doesn't win.
		{version: "10.10.8", date: day(11, 25)},
	}

	got, ok := floor(releases, cutoff, "")
	assert.True(t, ok)
	assert.Equal(t, release{version: "10.11.3", date: day(11, 18)}, got)
}

func TestFloorStartsAtTheMinimumWhereThatIsLater(t *testing.T) {
	cutoff := time.Date(2025, 11, 28, 0, 0, 0, 0, time.UTC)
	releases := []release{{version: "3.4.0", date: time.Date(2025, 9, 1, 0, 0, 0, 0, time.UTC)}}

	got, ok := floor(releases, cutoff, "3.5.0")
	assert.True(t, ok)
	assert.Equal(t, release{version: "3.5.0"}, got, "a zero date reads as the minimum")
}

func TestFloorOfAnAppYoungerThanTheWindowIsItsFirstRelease(t *testing.T) {
	cutoff := time.Date(2025, 11, 28, 0, 0, 0, 0, time.UTC)
	releases := []release{
		{version: "1.1.0", date: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)},
		{version: "1.0.0", date: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
	}

	got, ok := floor(releases, cutoff, "")
	assert.True(t, ok)
	assert.Equal(t, "1.0.0", got.version)

	_, ok = floor(nil, cutoff, "")
	assert.False(t, ok)
}
