package database

import "reflect"

// DeepCopy returns a copy of v that shares no mutable memory with it:
// pointers, slices (including datatypes.JSON byte slices) and maps are
// duplicated recursively. Unexported struct fields are copied by value.
//
// Caches of configuration objects hand out DeepCopy results, so a caller that
// modifies what it got cannot change what the next request sees. It works on
// any model without a per-type clone function that would silently miss a
// field added later.
func DeepCopy[T any](v T) T {
	src := reflect.ValueOf(&v).Elem()
	dst := reflect.New(src.Type()).Elem()
	deepCopyValue(dst, src)
	return dst.Interface().(T)
}

func deepCopyValue(dst, src reflect.Value) {
	switch src.Kind() {
	case reflect.Pointer:
		if src.IsNil() {
			return
		}
		p := reflect.New(src.Elem().Type())
		deepCopyValue(p.Elem(), src.Elem())
		dst.Set(p)
	case reflect.Slice:
		if src.IsNil() {
			return
		}
		s := reflect.MakeSlice(src.Type(), src.Len(), src.Len())
		for i := 0; i < src.Len(); i++ {
			deepCopyValue(s.Index(i), src.Index(i))
		}
		dst.Set(s)
	case reflect.Array:
		for i := 0; i < src.Len(); i++ {
			deepCopyValue(dst.Index(i), src.Index(i))
		}
	case reflect.Map:
		if src.IsNil() {
			return
		}
		m := reflect.MakeMapWithSize(src.Type(), src.Len())
		iter := src.MapRange()
		for iter.Next() {
			val := reflect.New(iter.Value().Type()).Elem()
			deepCopyValue(val, iter.Value())
			m.SetMapIndex(iter.Key(), val)
		}
		dst.Set(m)
	case reflect.Struct:
		// Copy the whole struct first, so unexported fields (which reflect
		// cannot set individually), such as time.Time's, carry over by value.
		dst.Set(src)
		for i := 0; i < src.NumField(); i++ {
			if dst.Field(i).CanSet() {
				deepCopyValue(dst.Field(i), src.Field(i))
			}
		}
	case reflect.Interface:
		if src.IsNil() {
			return
		}
		inner := reflect.New(src.Elem().Type()).Elem()
		deepCopyValue(inner, src.Elem())
		dst.Set(inner)
	default:
		dst.Set(src)
	}
}
