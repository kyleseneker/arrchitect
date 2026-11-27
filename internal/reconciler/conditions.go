package reconciler

import (
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// maxConditionMessage is the longest message the API server accepts on a
// condition; a longer one fails the whole status write.
const maxConditionMessage = 32768

// SetCondition sets a condition on a list of conditions.
func SetCondition(conditions *[]metav1.Condition, generation int64, condType string, status metav1.ConditionStatus, reason, message string) {
	if runes := []rune(message); len(runes) > maxConditionMessage {
		message = string(runes[:maxConditionMessage-3]) + "..."
	}
	condition := metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: generation,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
	}
	meta.SetStatusCondition(conditions, condition)
}
