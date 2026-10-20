package plugins

import (
	"strings"
	"unicode/utf8"

	"google.golang.org/protobuf/reflect/protoreflect"
)

// makeValidUTF8 replaces invalid UTF-8 in every string of m (fields, list
// items, map keys and values, nested messages) with U+FFFD and returns how
// many strings it changed.
//
// gRPC refuses to marshal a proto3 string that is not valid UTF-8, and a
// pulse that fails to send is kept for the next one. So one such string (a
// stored body cut inside a character, an upstream reply in another encoding,
// a client-chosen tool name) failed every later pulse as well, and the edge
// stopped delivering analytics, budget usage and compliance events for good.
func makeValidUTF8(m protoreflect.Message) int {
	fixed := 0
	clean := func(s string) (string, bool) {
		if utf8.ValidString(s) {
			return s, false
		}
		fixed++
		return strings.ToValidUTF8(s, "�"), true
	}
	m.Range(func(fd protoreflect.FieldDescriptor, v protoreflect.Value) bool {
		switch {
		case fd.IsList():
			list := v.List()
			for i := 0; i < list.Len(); i++ {
				switch fd.Kind() {
				case protoreflect.StringKind:
					if s, ok := clean(list.Get(i).String()); ok {
						list.Set(i, protoreflect.ValueOfString(s))
					}
				case protoreflect.MessageKind, protoreflect.GroupKind:
					fixed += makeValidUTF8(list.Get(i).Message())
				}
			}
		case fd.IsMap():
			mp := v.Map()
			type entry struct {
				oldKey protoreflect.MapKey
				newKey protoreflect.MapKey
				val    protoreflect.Value
			}
			var rekey []entry
			mp.Range(func(k protoreflect.MapKey, mv protoreflect.Value) bool {
				switch fd.MapValue().Kind() {
				case protoreflect.StringKind:
					if s, ok := clean(mv.String()); ok {
						mv = protoreflect.ValueOfString(s)
						mp.Set(k, mv)
					}
				case protoreflect.MessageKind, protoreflect.GroupKind:
					fixed += makeValidUTF8(mv.Message())
				}
				if fd.MapKey().Kind() == protoreflect.StringKind {
					if s, ok := clean(k.String()); ok {
						rekey = append(rekey, entry{k, protoreflect.ValueOfString(s).MapKey(), mv})
					}
				}
				return true
			})
			for _, e := range rekey {
				mp.Clear(e.oldKey)
				mp.Set(e.newKey, e.val)
			}
		case fd.Kind() == protoreflect.StringKind:
			if s, ok := clean(v.String()); ok {
				m.Set(fd, protoreflect.ValueOfString(s))
			}
		case fd.Kind() == protoreflect.MessageKind || fd.Kind() == protoreflect.GroupKind:
			fixed += makeValidUTF8(v.Message())
		}
		return true
	})
	return fixed
}
