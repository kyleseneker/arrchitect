// Package secretref reads the Secrets a resource's spec names, and requeues
// the resources that name a Secret when it changes.
package secretref

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"sigs.k8s.io/controller-runtime/pkg/client"

	commonv1alpha1 "github.com/kyleseneker/arrchitect/api/common/v1alpha1"
	"github.com/kyleseneker/arrchitect/internal/failure"
	"github.com/kyleseneker/arrchitect/internal/secrets"
)

// ResolveConfigFields returns a copy of fields with any valueFrom reference
// resolved into a literal value, so secrets stay out of the CR.
func ResolveConfigFields(ctx context.Context, c client.Reader, namespace, owner string, fields []commonv1alpha1.ConfigField) ([]commonv1alpha1.ConfigField, error) {
	if len(fields) == 0 {
		return fields, nil
	}
	out := make([]commonv1alpha1.ConfigField, len(fields))
	copy(out, fields)
	for i := range out {
		if out[i].ValueFrom == (commonv1alpha1.ValueSource{}) {
			continue
		}
		if out[i].Value != nil {
			return nil, failure.InvalidConfig(fmt.Errorf("%s field %q: value and valueFrom are mutually exclusive", owner, out[i].Name))
		}
		val, err := secrets.ResolveSecretKeyRef(ctx, c, namespace, out[i].ValueFrom.SecretKeyRef)
		if err != nil {
			return nil, fmt.Errorf("%s field %q: %w", owner, out[i].Name, err)
		}
		raw, err := json.Marshal(val)
		if err != nil {
			return nil, fmt.Errorf("%s field %q: encoding secret value: %w", owner, out[i].Name, err)
		}
		out[i].Value = &commonv1alpha1.FieldValue{Raw: raw}
		out[i].ValueFrom = commonv1alpha1.ValueSource{}
	}
	return out, nil
}

// FieldValues resolves fields into values by name, reading the ones that come
// from Secrets.
func FieldValues(ctx context.Context, c client.Reader, namespace, owner string, fields []commonv1alpha1.ConfigField) (map[string]any, error) {
	resolved, err := ResolveConfigFields(ctx, c, namespace, owner, fields)
	if err != nil {
		return nil, err
	}
	out := make(map[string]any, len(resolved))
	for _, f := range resolved {
		if f.Value != nil {
			out[f.Name] = f.Value.ToInterface()
		}
	}
	return out, nil
}

// ResolveItemFields returns a copy of items with each item's field Secret
// references resolved. what names the kind of item in errors, and fields
// returns an item's name and its fields.
func ResolveItemFields[T any](
	ctx context.Context, c client.Reader, namespace, what string, items []T,
	fields func(*T) (string, *[]commonv1alpha1.ConfigField),
) ([]T, error) {
	out := slices.Clone(items)
	for i := range out {
		name, f := fields(&out[i])
		resolved, err := ResolveConfigFields(ctx, c, namespace, what+" "+name, *f)
		if err != nil {
			return nil, err
		}
		*f = resolved
	}
	return out, nil
}
