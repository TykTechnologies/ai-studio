package proxy

import "github.com/TykTechnologies/midsommar/v2/services"

// nativeStoreOf returns the native resource ledger behind service, or nil
// when it has none (native API operations then answer 501).
func nativeStoreOf(service services.ServiceInterface) services.NativeResourceStore {
	if s, ok := service.(interface {
		NativeResourceStore() services.NativeResourceStore
	}); ok {
		return s.NativeResourceStore()
	}
	return nil
}
