package reconciler

import (
	"reflect"
	"slices"
	"strings"
	"sync"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ownerKey names obj in state the operator keeps between passes: the secrets
// it wrote and ownership no status write has recorded yet. It includes the
// UID, so a resource deleted and created again under the same name starts
// afresh.
func ownerKey(obj client.Object) string {
	return ownerPrefix(obj) + string(obj.GetUID())
}

// ownerPrefix starts the ownerKey of every incarnation of obj.
func ownerPrefix(obj client.Object) string {
	return reflect.TypeOf(obj).Elem().Name() + "/" + obj.GetNamespace() + "/" + obj.GetName() + "/"
}

// unrecorded holds, by ownerKey, the ownership of a pass whose status write
// failed. A resource the operator created but no longer tracks could never be
// pruned or cleaned up, so it stays owned here until a write records it.
var unrecorded = struct {
	sync.Mutex
	managed map[string]map[string][]string
}{managed: map[string]map[string][]string{}}

// owned is what obj owns: the ownership of its last pass if no status write
// has recorded it yet, else what its status records.
func owned(obj ConfigResource) map[string][]string {
	unrecorded.Lock()
	defer unrecorded.Unlock()
	if managed, ok := unrecorded.managed[ownerKey(obj)]; ok {
		return cloneManaged(managed)
	}
	return obj.GetConfigStatus().GetManagedResources()
}

// remember keeps the ownership of obj's pass until a status write records it.
func remember(obj ConfigResource, managed map[string][]string, recorded bool) {
	unrecorded.Lock()
	defer unrecorded.Unlock()
	if recorded {
		delete(unrecorded.managed, ownerKey(obj))
	} else {
		unrecorded.managed[ownerKey(obj)] = cloneManaged(managed)
	}
}

// forgetOwnership drops what unrecorded holds for every incarnation of obj.
func forgetOwnership(obj client.Object) {
	prefix := ownerPrefix(obj)
	unrecorded.Lock()
	defer unrecorded.Unlock()
	for key := range unrecorded.managed {
		if strings.HasPrefix(key, prefix) {
			delete(unrecorded.managed, key)
		}
	}
}

func cloneManaged(managed map[string][]string) map[string][]string {
	out := make(map[string][]string, len(managed))
	for kind, names := range managed {
		out[kind] = slices.Clone(names)
	}
	return out
}
