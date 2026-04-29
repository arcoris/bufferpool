// Package multierr accumulates multiple independent errors.
//
// The package is used when cleanup or validation should continue after the
// first failure, for example when closing several owned resources or collecting
// independent configuration errors. It preserves all observed failures while
// letting callers return a single error value.
//
// multierr does not classify errors by itself and does not import the root
// bufferpool package. Callers remain responsible for wrapping or classifying the
// accumulated result when a public error class is required.
package multierr
