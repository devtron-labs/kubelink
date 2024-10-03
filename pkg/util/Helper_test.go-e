/*
 * Copyright (c) 2024. Devtron Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package util

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestMapSubset1(t *testing.T) {
	superSet := map[string]string{"a": "b", "c": "d", "e": "f"}
	subSet := map[string]string{"a": "b", "e": "f"}
	assert.True(t, IsMapSubset(superSet, subSet))
}

func TestMapSubset2(t *testing.T) {
	superSet := map[string]string{"a": "b", "c": "d", "e": "f"}
	subSet := map[string]string{"name": "hello"}
	assert.False(t, IsMapSubset(superSet, subSet))
}

func TestMapSubset3(t *testing.T) {
	superSet := map[string]string{"a": "b", "c": "d", "e": "f"}
	subSet := map[string]string{"a": "b", "e": "ff"}
	assert.False(t, IsMapSubset(superSet, subSet))
}
