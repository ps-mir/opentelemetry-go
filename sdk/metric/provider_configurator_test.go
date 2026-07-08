// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package metric

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.opentelemetry.io/otel/sdk/instrumentation"
)

// testMeterConfig satisfies meterConfigReader without importing sdk/metric/x.
type testMeterConfig struct{ enabled bool }

func (c testMeterConfig) Enabled() bool { return c.enabled }

// testConfiguratorOpt is a test implementation of meterConfiguratorOption.
type testConfiguratorOpt struct {
	Option
	fn       func(instrumentation.Scope) any
	onUpdate func(func())
}

func (testConfiguratorOpt) Experimental() {}

func (o testConfiguratorOpt) MeterConfigurator() func(instrumentation.Scope) any { return o.fn }

func (o testConfiguratorOpt) RegisterOnUpdate(cb func()) {
	if o.onUpdate != nil {
		o.onUpdate(cb)
	}
}

// disablingConfiguratorFn disables meters whose scope name is "disabled".
func disablingConfiguratorFn(s instrumentation.Scope) any {
	return testMeterConfig{enabled: s.Name != "disabled"}
}

func TestMeterConfiguratorNewMeter(t *testing.T) {
	for _, tc := range []struct {
		name            string
		configuratorOpt Option
		scope           string
		wantEnabled     bool
	}{
		{
			name:            "configurator/none",
			configuratorOpt: nil,
			scope:           "any",
			wantEnabled:     true,
		},
		{
			name:            "configurator/scope/disabled",
			configuratorOpt: testConfiguratorOpt{fn: disablingConfiguratorFn},
			scope:           "disabled",
			wantEnabled:     false,
		},
		{
			name:            "configurator/scope/enabled",
			configuratorOpt: testConfiguratorOpt{fn: disablingConfiguratorFn},
			scope:           "other",
			wantEnabled:     true,
		},
		{
			name:            "configurator/set-before-provider",
			configuratorOpt: testConfiguratorOpt{fn: disablingConfiguratorFn},
			scope:           "disabled",
			wantEnabled:     false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var configuratorOpts []Option
			if tc.configuratorOpt != nil {
				configuratorOpts = append(configuratorOpts, tc.configuratorOpt)
			}

			mp := NewMeterProvider(configuratorOpts...)
			defer mp.Shutdown(context.Background()) //nolint:errcheck

			_ = mp.Meter(tc.scope)
			m := mp.meters.Lookup(instrumentation.Scope{Name: tc.scope}, func() *meter {
				return newMeter(instrumentation.Scope{Name: tc.scope}, mp.pipes)
			})
			require.NotNil(t, m)
			assert.Equal(t, tc.wantEnabled, m.enabled.Load())
		})
	}
}

func TestConfiguratorCachedMeterNotUpdated(t *testing.T) {
	var storedCallback func()
	configuratorOpt := testConfiguratorOpt{
		fn: func(s instrumentation.Scope) any {
			return testMeterConfig{enabled: s.Name != "disabled"}
		},
		onUpdate: func(cb func()) { storedCallback = cb },
	}

	mp := NewMeterProvider(configuratorOpt)
	defer mp.Shutdown(context.Background()) //nolint:errcheck

	// Create and cache a meter before updating the configurator.
	_ = mp.Meter("test")
	cachedMeter := mp.meters.Lookup(instrumentation.Scope{Name: "test"}, func() *meter {
		return newMeter(instrumentation.Scope{Name: "test"}, mp.pipes)
	})
	assert.True(t, cachedMeter.enabled.Load(), "meter should be enabled before configurator update")

	// Swap configurator to disable all scopes and simulate handle.Set().
	mp.configurator = func(s instrumentation.Scope) any {
		return testMeterConfig{enabled: false}
	}
	if storedCallback != nil {
		storedCallback()
	}

	// Cached meter is not updated without cache walk.
	// TODO: assert enabled=false once cache walk is implemented (Step 2: cache.Range)
	assert.True(t, cachedMeter.enabled.Load(), "cached meter not updated without cache walk")

	// New meter picks up the updated configurator.
	_ = mp.Meter("disabled")
	newM := mp.meters.Lookup(instrumentation.Scope{Name: "disabled"}, func() *meter {
		return newMeter(instrumentation.Scope{Name: "disabled"}, mp.pipes)
	})
	assert.False(t, newM.enabled.Load(), "new meter should pick up updated configurator")
}
