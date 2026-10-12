// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package render

import (
	"reflect"
	"strings"
	"unicode"

	"github.com/DataDog/datadog-operator/pkg/plugin/dashboard"
)

// oneLine makes a cluster-provided string safe to print on one line:
// whitespace runs collapse to one space, other control characters and
// invalid UTF-8 become repl.
func oneLine(s string, repl rune) string {
	s = strings.ToValidUTF8(s, string(repl))
	s = strings.Map(func(r rune) rune {
		switch {
		case unicode.IsSpace(r):
			return ' '
		case unicode.IsControl(r):
			return repl
		default:
			return r
		}
	}, s)
	return strings.Join(strings.Fields(s), " ")
}

// sanitizeView returns a deep copy of v with every string passed through
// oneLine, so that the renderer can print any field as is. The original View
// is left untouched; the JSON output escapes control characters itself.
func sanitizeView(v dashboard.View, repl rune) dashboard.View {
	rv := reflect.New(reflect.TypeFor[dashboard.View]()).Elem()
	rv.Set(reflect.ValueOf(v))
	sanitizeValue(rv, repl)
	return rv.Interface().(dashboard.View)
}

// sanitizeValue cleans the strings reachable from the settable value v,
// copying pointers, slices and maps so that nothing is shared with the
// original. Unexported fields are left alone.
func sanitizeValue(v reflect.Value, repl rune) {
	switch v.Kind() {
	case reflect.String:
		v.SetString(oneLine(v.String(), repl))
	case reflect.Struct:
		for _, f := range v.Fields() {
			if f.CanSet() {
				sanitizeValue(f, repl)
			}
		}
	case reflect.Pointer:
		if v.IsNil() {
			return
		}
		p := reflect.New(v.Type().Elem())
		p.Elem().Set(v.Elem())
		sanitizeValue(p.Elem(), repl)
		v.Set(p)
	case reflect.Slice:
		if v.IsNil() {
			return
		}
		s := reflect.MakeSlice(v.Type(), v.Len(), v.Len())
		reflect.Copy(s, v)
		for i := range s.Len() {
			sanitizeValue(s.Index(i), repl)
		}
		v.Set(s)
	case reflect.Map:
		if v.IsNil() {
			return
		}
		m := reflect.MakeMapWithSize(v.Type(), v.Len())
		iter := v.MapRange()
		for iter.Next() {
			k := reflect.New(v.Type().Key()).Elem()
			k.Set(iter.Key())
			sanitizeValue(k, repl)
			e := reflect.New(v.Type().Elem()).Elem()
			e.Set(iter.Value())
			sanitizeValue(e, repl)
			m.SetMapIndex(k, e)
		}
		v.Set(m)
	}
}
