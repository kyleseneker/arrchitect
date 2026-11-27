package reconciler

import (
	"testing"

	"github.com/stretchr/testify/assert"

	pvrv1alpha1 "github.com/kyleseneker/arrchitect/api/pvr/v1alpha1"
)

// What a pass created stays owned when its status write fails, until a later
// write records it.
func TestOwnershipLastsUntilAStatusWriteRecordsIt(t *testing.T) {
	obj := &pvrv1alpha1.Sonarr{Name: t.Name(), Namespace: "media", UID: "uid-1"}
	obj.Status.SetManagedResources(map[string][]string{"tags": {"a"}})
	assert.Equal(t, map[string][]string{"tags": {"a"}}, owned(obj))

	remember(obj, map[string][]string{"tags": {"a", "b"}}, false)
	assert.Equal(t, map[string][]string{"tags": {"a", "b"}}, owned(obj), "the unrecorded ownership was lost")

	remember(obj, map[string][]string{"tags": {"a", "b"}}, true)
	assert.Equal(t, map[string][]string{"tags": {"a"}}, owned(obj), "a recorded pass reads its status again")
}

func TestRememberedOwnershipIsACopy(t *testing.T) {
	obj := &pvrv1alpha1.Sonarr{Name: t.Name(), Namespace: "media", UID: "uid-1"}
	managed := map[string][]string{"tags": {"a"}}
	remember(obj, managed, false)
	managed["tags"][0] = "changed"
	got := owned(obj)
	got["tags"][0] = "changed again"

	assert.Equal(t, map[string][]string{"tags": {"a"}}, owned(obj))
	forgetOwnership(obj)
}

func TestARecreatedResourceStartsWithoutTheOldOnesOwnership(t *testing.T) {
	old := &pvrv1alpha1.Sonarr{Name: t.Name(), Namespace: "media", UID: "first"}
	remember(old, map[string][]string{"tags": {"a"}}, false)

	recreated := &pvrv1alpha1.Sonarr{Name: t.Name(), Namespace: "media", UID: "second"}
	assert.Empty(t, owned(recreated), "a new UID starts from its own status")

	forgetOwnership(&pvrv1alpha1.Sonarr{Name: t.Name(), Namespace: "media"})
	assert.Empty(t, owned(old), "a deleted resource's ownership is dropped")
}
