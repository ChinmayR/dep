package main

import (
	"testing"

	"github.com/golang/dep/uber"
	"github.com/stretchr/testify/assert"
)

func TestMaxThreads_UnsetShouldGetDefault(t *testing.T) {
	defer uber.SetAndUnsetEnvVar(MaxThreadsEnvVar, "")()

	wantVal := defaultMaxThreads
	gotVal := getMaxThreadsFromEnvVar()

	assert.Equal(t, wantVal, gotVal)
}

func TestMaxThreads_SetShouldGetVal(t *testing.T) {
	defer uber.SetAndUnsetEnvVar(MaxThreadsEnvVar, "10")()

	wantVal := 10
	gotVal := getMaxThreadsFromEnvVar()

	assert.Equal(t, wantVal, gotVal)
}

func TestMaxThreads_GarbageShouldReturnDefault(t *testing.T) {
	defer uber.SetAndUnsetEnvVar(MaxThreadsEnvVar, "garbageVal")()

	wantVal := defaultMaxThreads
	gotVal := getMaxThreadsFromEnvVar()

	assert.Equal(t, wantVal, gotVal)
}
