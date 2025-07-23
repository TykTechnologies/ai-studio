package aigateway

// This file previously contained duplicate interfaces that have been unified.
// All interfaces now use the unified services.ServiceInterface from services/simple_interface.go
// to eliminate duplication and confusion.
//
// The unified interface approach provides:
// - Single source of truth for service operations
// - Clean architecture without interface duplication
// - Analytics compatibility across all implementations
// - Flexible OAuth support with graceful degradation
