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
