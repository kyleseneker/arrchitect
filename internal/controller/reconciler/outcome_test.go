package reconciler

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	commonv1alpha1 "github.com/kyleseneker/arrchitect/api/common/v1alpha1"
	pvrv1alpha1 "github.com/kyleseneker/arrchitect/api/pvr/v1alpha1"
	"github.com/kyleseneker/arrchitect/internal/controller/appref"
	"github.com/kyleseneker/arrchitect/internal/failure"
	"github.com/kyleseneker/arrchitect/internal/httpapi"
	"github.com/kyleseneker/arrchitect/internal/secrets"
)

// The failure policy: what each kind of failure reads as, whether it's
// stalled, and whether it backs off rather than waiting for the interval.
func TestEachKindOfFailureHasItsOutcome(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want outcome
	}{
		{failure.Unreachable.Errorf("connection refused"), outcome{reason: ReasonAppUnreachable, backoff: true}},
		{failure.Transient.Errorf("reading secret: timeout"), outcome{reason: ReasonSyncFailed, backoff: true}},
		{failure.Unauthorized.Errorf("401"), outcome{reason: ReasonUnauthorized, stalled: true}},
		{failure.SecretNotFound.Errorf("missing"), outcome{reason: ReasonSecretNotFound}},
		{failure.ReferenceNotFound.Errorf("missing"), outcome{reason: ReasonReferenceNotFound}},
		{failure.InvalidConfig.Errorf("bad URL"), outcome{reason: ReasonInvalidConfig, stalled: true}},
		{failure.NameConflict.Errorf("taken"), outcome{reason: ReasonNameConflict, stalled: true}},
		{&httpapi.APIError{StatusCode: 400}, outcome{reason: ReasonSyncFailed}},
	} {
		t.Run(tc.want.reason+"/"+tc.err.Error(), func(t *testing.T) {
			assert.Equal(t, tc.want, outcomeOf([]error{fmt.Errorf("settings: %w", tc.err)}))
		})
	}
}

// Failures that are all of one kind, joined or not, take its outcome. Any mix
// is refused, so a write the app refused isn't sent again sooner because
// something else is missing.
func TestSeveralFailuresShareAnOutcomeOnlyWhenTheyShareAKind(t *testing.T) {
	missing := failure.SecretNotFound.Errorf("secret %q not found", "keys")
	refusedWrite := errors.New("flow(a): x")
	for _, tc := range []struct {
		name string
		errs []error
		want string
	}{
		{"failures that share a kind", []error{fmt.Errorf("settings: %w", missing), fmt.Errorf("library(a): %w", missing)}, ReasonSecretNotFound},
		{"failures of different kinds", []error{fmt.Errorf("settings: %w", missing), refusedWrite}, ReasonSyncFailed},
		{"joined failures of different kinds", []error{
			fmt.Errorf("general: %w", failure.Join(fmt.Errorf("series: %w", missing), failure.Unauthorized.Errorf("401"))),
		}, ReasonSyncFailed},
		{"joined failures that share a kind", []error{
			fmt.Errorf("sonarr: %w", failure.Join(fmt.Errorf("a: %w", missing), fmt.Errorf("b: %w", missing))),
		}, ReasonSecretNotFound},
		{"a spec problem around joined failures", []error{
			failure.InvalidConfig.Wrap(failure.Join(errors.New("a"), failure.Unauthorized.Errorf("401"))),
		}, ReasonInvalidConfig},
		{"refused writes", []error{refusedWrite, errors.New("flow(b): y")}, ReasonSyncFailed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, outcomeOf(tc.errs).reason)
		})
	}
}

// Following an appRef and reading the Secrets it names fail with the kind
// that says what to fix.
func TestAppRefFailuresSayWhatToFix(t *testing.T) {
	s := runtime.NewScheme()
	require.NoError(t, scheme.AddToScheme(s))
	require.NoError(t, pvrv1alpha1.AddToScheme(s))
	c := fake.NewClientBuilder().WithScheme(s).WithObjects(
		&pvrv1alpha1.Sonarr{Name: "nourl", Namespace: "media", Spec: pvrv1alpha1.SonarrSpec{
			Connection: commonv1alpha1.AppConnection{URL: "http://:8989"},
		}},
		&pvrv1alpha1.Sonarr{Name: "sonarr", Namespace: "media", Spec: pvrv1alpha1.SonarrSpec{
			Connection: commonv1alpha1.AppConnection{URL: "http://sonarr:8989"},
		}},
	).Build()
	resolve := func(apps appref.Apps, kind, name string) error {
		_, _, err := appref.ResolveAddress(context.Background(), c, apps, "media", commonv1alpha1.AppRef{Kind: kind, Name: name})
		return fmt.Errorf("download client %q appRef: %w", "client", err)
	}
	_, secretErr := secrets.ResolveSecretKeyRef(context.Background(), c, "media", commonv1alpha1.SecretKeyRef{Name: "missing", Key: "key"})

	for _, tc := range []struct {
		name string
		err  error
		want string
	}{
		{"missing resource", resolve(nil, "Sonarr", "gone"), ReasonReferenceNotFound},
		{"unsupported kind", resolve(nil, "Tdarr", "tdarr"), ReasonInvalidConfig},
		{"app the operator doesn't run", resolve(appref.Apps{"radarr": true}, "Sonarr", "sonarr"), ReasonInvalidConfig},
		{"no usable URL", resolve(nil, "Sonarr", "nourl"), ReasonInvalidConfig},
		{"missing Secret", fmt.Errorf("download client %q password: %w", "client", secretErr), ReasonSecretNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Error(t, tc.err)
			assert.Equal(t, tc.want, outcomeOf([]error{tc.err}).reason, tc.err.Error())
		})
	}
}
