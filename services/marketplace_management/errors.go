package marketplace_management

import "errors"

var (
	// ErrEnterpriseOnly is returned when marketplace management is attempted in Community Edition
	ErrEnterpriseOnly = errors.New("multiple marketplace management is only available in Enterprise Edition")

	// ErrMarketplaceNotFound is returned when a marketplace ID doesn't exist
	ErrMarketplaceNotFound = errors.New("marketplace not found")

	// ErrCannotRemoveDefault is returned when attempting to remove the default marketplace
	ErrCannotRemoveDefault = errors.New("cannot remove the default marketplace")

	// ErrCannotDeactivateDefault is returned when attempting to deactivate the default marketplace
	ErrCannotDeactivateDefault = errors.New("cannot deactivate the default marketplace")

	// ErrInvalidURL is returned when a marketplace URL is invalid
	ErrInvalidURL = errors.New("invalid marketplace URL")

	// ErrDuplicateURL is returned when attempting to add a marketplace that already exists
	ErrDuplicateURL = errors.New("marketplace with this URL already exists")

	// ErrMarketplaceUnreachable is returned when a marketplace URL cannot be accessed
	ErrMarketplaceUnreachable = errors.New("marketplace URL is unreachable")

	// ErrInvalidIndexFormat is returned when the marketplace index.yaml is malformed
	ErrInvalidIndexFormat = errors.New("marketplace index.yaml format is invalid")
)
