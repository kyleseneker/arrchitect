// Package contract provides a shared check that a payload builder emits a key
// for every field its spec declares.
package contract

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"

	commonv1alpha1 "github.com/kyleseneker/arrchitect/api/common/v1alpha1"
)

// AssertEveryFieldEmitted fills every field of spec, runs build, and fails if the
// resulting payload has fewer keys than the spec has fields. Apps accept unknown
// keys and ignore missing ones without complaint, so a field the builder forgets
// is silently inert rather than an error. An appRef isn't counted: it is
// resolved into the spec's other fields before a payload is built.
func AssertEveryFieldEmitted[T any, V any](t *testing.T, spec *T, build func(*T) map[string]V, fixups ...func(*T)) {
	t.Helper()

	v := reflect.ValueOf(spec).Elem()
	fields := v.NumField()
	for _, f := range v.Fields() {
		if t := f.Type(); t == reflect.TypeFor[commonv1alpha1.AppRef]() || t == reflect.TypeFor[*commonv1alpha1.AppRef]() {
			fields--
			continue
		}
		switch f.Kind() {
		case reflect.Pointer:
			f.Set(reflect.New(f.Type().Elem()))
		case reflect.String:
			f.SetString("x")
		case reflect.Int, reflect.Int32, reflect.Int64:
			f.SetInt(1)
		}
	}
	for _, fix := range fixups {
		fix(spec)
	}

	got := build(spec)
	if !assert.Len(t, got, fields,
		"%T declares %d fields but the builder emitted %d keys; an unmapped field is silently dropped by the target app",
		spec, fields, len(got)) {
		for i := range v.NumField() {
			t.Logf("  spec field: %s", v.Type().Field(i).Name)
		}
		for k := range got {
			t.Logf("  emitted key: %s", k)
		}
	}
}
