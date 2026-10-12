package proxy

import (
	"testing"

	"github.com/TykTechnologies/midsommar/v2/services"
	"github.com/stretchr/testify/assert"
)

type serviceWithoutNativeStore struct{ services.ServiceInterface }

type serviceWithNativeStore struct {
	services.ServiceInterface
	store services.NativeResourceStore
}

func (s serviceWithNativeStore) NativeResourceStore() services.NativeResourceStore { return s.store }

func TestNativeStoreOf(t *testing.T) {
	assert.Nil(t, nativeStoreOf(nil))
	assert.Nil(t, nativeStoreOf(serviceWithoutNativeStore{}), "a service without the accessor has no store")
	assert.Nil(t, nativeStoreOf(&services.Service{}), "Studio without a database has no store")

	store := services.NewNativeResourceStore(setupTestDB(t), nil)
	assert.Same(t, store, nativeStoreOf(serviceWithNativeStore{store: store}))
}
