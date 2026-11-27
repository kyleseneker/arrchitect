package reconciler

import (
	"reflect"
	"strings"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"

	commonv1alpha1 "github.com/kyleseneker/arrchitect/api/common/v1alpha1"
)

// maxRetryDelay caps the wait between retries of an app that keeps failing.
const maxRetryDelay = 10 * time.Minute

// jitter is the fraction a delay is stretched by at random, so resources that
// failed or synced together don't all reach their app at once.
const jitter = 0.1

// failures counts each resource's failures in a row to reach its app.
var failures = struct {
	sync.Mutex
	count map[string]int
}{count: map[string]int{}}

// RetryAfter is how long obj waits after another failure to reach its app:
// base, doubled for each failure in a row before this one, up to
// maxRetryDelay. A base of zero means no retry.
func RetryAfter(obj client.Object, base time.Duration) time.Duration {
	if base <= 0 {
		return 0
	}
	key := OwnerKey(obj)
	failures.Lock()
	n := failures.count[key]
	dropOthers(obj, key)
	failures.count[key] = n + 1
	failures.Unlock()
	delay := maxRetryDelay
	if n < 16 && base<<n < maxRetryDelay {
		delay = base << n
	}
	return wait.Jitter(delay, jitter)
}

// Reached clears obj's failures once its app answers.
func Reached(obj client.Object) {
	forget(obj)
}

// forget drops the failures of every resource that had obj's kind, namespace
// and name.
func forget(obj client.Object) {
	failures.Lock()
	dropOthers(obj, "")
	failures.Unlock()
}

// dropOthers drops the entries for obj's kind, namespace and name other than
// keep, such as one a resource deleted and created again left behind. The
// caller holds the lock.
func dropOthers(obj client.Object, keep string) {
	prefix := ownerPrefix(obj)
	for key := range failures.count {
		if key != keep && strings.HasPrefix(key, prefix) {
			delete(failures.count, key)
		}
	}
}

// NextSync is when a resource that reached its app next reconciles.
func NextSync(rc *commonv1alpha1.ReconcileConfig) time.Duration {
	return wait.Jitter(ReconcileInterval(rc), jitter)
}

// OwnerKey names obj in state the operator keeps between passes: its failures
// in a row and the secrets it wrote. It includes the UID, so a resource
// deleted and created again under the same name starts afresh.
func OwnerKey(obj client.Object) string {
	return ownerPrefix(obj) + string(obj.GetUID())
}

// ownerPrefix starts the OwnerKey of every incarnation of obj.
func ownerPrefix(obj client.Object) string {
	return reflect.TypeOf(obj).Elem().Name() + "/" + obj.GetNamespace() + "/" + obj.GetName() + "/"
}
