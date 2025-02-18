package proxy

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/TykTechnologies/midsommar/v2/services"
)

func TestProxySetup(t *testing.T) {
	db, cancel := setupTest(t)
	defer tearDownTest(db, cancel)

	service := services.NewService(db)
	notificationSvc := services.NewTestNotificationService(db)
	budgetService := services.NewBudgetService(db, notificationSvc)

	config := &Config{Port: 8080}
	p := NewProxy(service, config, budgetService)
	assert.NotNil(t, p)
}

func TestConcurrentAccess(t *testing.T) {
	db, cancel := setupTest(t)
	defer tearDownTest(db, cancel)

	service := services.NewService(db)
	notificationSvc := services.NewTestNotificationService(db)
	budgetService := services.NewBudgetService(db, notificationSvc)
	proxy := NewProxy(service, &Config{Port: 9999}, budgetService)

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = proxy.llms
			_ = proxy.datasources
		}()
	}
	wg.Wait()
}
