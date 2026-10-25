package models

// KeepAllowInternalHost puts original's allow_internal_host back in modified
// (an object hook's result): the per-asset "Allow internal host" switch is
// an administrator's alone (epic w4e), so a plugin hook can never set or
// clear it. Objects without the switch are left alone.
func KeepAllowInternalHost(original, modified any) {
	switch m := modified.(type) {
	case *LLM:
		if o, ok := original.(*LLM); ok && o != nil && m != nil {
			m.AllowInternalHost = o.AllowInternalHost
		}
	case *Tool:
		if o, ok := original.(*Tool); ok && o != nil && m != nil {
			m.AllowInternalHost = o.AllowInternalHost
		}
	case *Datasource:
		if o, ok := original.(*Datasource); ok && o != nil && m != nil {
			m.AllowInternalHost = o.AllowInternalHost
		}
	}
}
