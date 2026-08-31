// Licensed to the Apache Software Foundation (ASF) under one
// or more contributor license agreements. See the NOTICE file
// distributed with this work for additional information regarding
// copyright ownership. The ASF licenses this file to You under the Apache
// License, Version 2.0 (the "License"); you may not use this file except in
// compliance with the License. You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package provider

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRetryLookupUntilWaitsForStaleStateToConverge(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	calls := 0

	value, found, converged, err := retryLookupUntil(ctx, func(context.Context) (string, bool, error) {
		calls++
		if calls < 3 {
			return "stale", true, nil
		}

		return "planned", true, nil
	}, func(value string) bool {
		return value == "planned"
	})

	require.NoError(t, err)
	assert.True(t, found)
	assert.True(t, converged)
	assert.Equal(t, "planned", value)
	assert.Equal(t, 3, calls)
}

func TestRetryLookupUntilReturnsLastStaleStateAtDeadline(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 75*time.Millisecond)
	defer cancel()

	value, found, converged, err := retryLookupUntil(ctx, func(context.Context) (string, bool, error) {
		return "stale", true, nil
	}, func(value string) bool {
		return value == "planned"
	})

	require.NoError(t, err)
	assert.True(t, found)
	assert.False(t, converged)
	assert.Equal(t, "stale", value)
}
