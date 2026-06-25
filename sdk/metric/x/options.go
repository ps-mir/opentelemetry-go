// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package x

import (
	"sync/atomic"

	"go.opentelemetry.io/otel/sdk/instrumentation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

// MeterConfiguratorHandle holds a [MeterConfigurator] that can be updated at
// runtime. Pass it to [WithMeterConfigurator] at [sdkmetric.MeterProvider]
// construction. Calls to [MeterConfiguratorHandle.Set] are reflected
// immediately across all existing meters via a synchronous cache walk.
type MeterConfiguratorHandle struct {
	configurator atomic.Pointer[MeterConfigurator]
	onUpdate     atomic.Pointer[func()] // To avoid race between handle.Set() and RegisterOnUpdate
}

// NewMeterConfiguratorHandle returns a new [MeterConfiguratorHandle] with no
// configurator set.
func NewMeterConfiguratorHandle() *MeterConfiguratorHandle {
	return &MeterConfiguratorHandle{}
}

// Set updates the [MeterConfigurator] and triggers a synchronous cache walk on
// the [sdkmetric.MeterProvider] registered via [WithMeterConfigurator].
func (h *MeterConfiguratorHandle) Set(fn MeterConfigurator) {
	h.configurator.Store(&fn)
	if cb := h.onUpdate.Load(); cb != nil {
		(*cb)()
	}
}

type meterConfiguratorProviderOption struct {
	// nil embed; skip guard in newConfig prevents apply from being called.
	sdkmetric.Option
	handle *MeterConfiguratorHandle
}

// Experimental marks this as an experimental option so the skip guard in
// newConfig skips calling the nil embedded apply.
func (meterConfiguratorProviderOption) Experimental() {}

// WithMeterConfigurator returns an [sdkmetric.Option] that wires a
// [MeterConfiguratorHandle] into a [sdkmetric.MeterProvider]. The handle must
// be passed at construction; runtime configurator updates via
// [MeterConfiguratorHandle.Set] are only supported when a handle is registered
// here. Providers created without this option cannot have a configurator added
// later.
func WithMeterConfigurator(h *MeterConfiguratorHandle) sdkmetric.Option {
	return meterConfiguratorProviderOption{handle: h}
}

// MeterConfigurator returns a closure over the handle so sdk/metric can call
// it via duck-type without importing this package.
func (o meterConfiguratorProviderOption) MeterConfigurator() func(instrumentation.Scope) any {
	return func(s instrumentation.Scope) any {
		if p := o.handle.configurator.Load(); p != nil {
			return (*p)(s)
		}
		return MeterConfig{}
	}
}

// RegisterOnUpdate is called by sdk/metric during [sdkmetric.NewMeterProvider]
// to register the cache walk callback. Called once at construction; subsequent
// [MeterConfiguratorHandle.Set] calls trigger it.
func (o meterConfiguratorProviderOption) RegisterOnUpdate(fn func()) {
	o.handle.onUpdate.Store(&fn)
}
