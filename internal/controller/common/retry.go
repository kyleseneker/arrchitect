package common

import (
	"reflect"
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
	key := retryKey(obj)
	failures.Lock()
	n := failures.count[key]
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
	failures.Lock()
	delete(failures.count, retryKey(obj))
	failures.Unlock()
}

// NextSync is when a resource that reached its app next reconciles.
func NextSync(rc *commonv1alpha1.ReconcileConfig) time.Duration {
	return wait.Jitter(ReconcileInterval(rc), jitter)
}

func retryKey(obj client.Object) string {
	return reflect.TypeOf(obj).Elem().Name() + "/" + obj.GetNamespace() + "/" + obj.GetName()
}
