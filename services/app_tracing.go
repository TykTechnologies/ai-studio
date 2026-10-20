package services

import (
	"github.com/TykTechnologies/midsommar/v2/helpers"
	"github.com/TykTechnologies/midsommar/v2/models"
)

// WithAppTracing sets the App's agent tracing overrides. Without it a new
// App inherits every global setting and an update keeps the stored ones.
func WithAppTracing(t models.AppTracingSettings) AppOption {
	return func(o *appOptions) { o.tracing = &t }
}

// applyAppTracing validates and applies the tracing option, if any.
func applyAppTracing(app *models.App, o appOptions) error {
	if o.tracing == nil {
		return nil
	}
	if err := o.tracing.Validate(); err != nil {
		return helpers.NewBadRequestError(err.Error())
	}
	app.Tracing = *o.tracing
	return nil
}
