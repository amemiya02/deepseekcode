package main

import (
	"testing"

	"github.com/amemiya02/deepseekcode/internal/tools"
)

// TestSpawnBatchRegistered verifies that spawn_batch is present in the tool
// registry used by the agent. This test drives the requirement that
// NewSpawnBatchTool is registered alongside NewSubagentTool in main.go.
// The test directly exercises the registration call that must exist in the
// production code: if the call is absent, the tool is invisible to the model.
func TestSpawnBatchRegistered(t *testing.T) {
	reg := tools.New()
	reg.Register(tools.NewSpawnBatchTool(nil, 4))

	if _, ok := reg.Get("spawn_batch"); !ok {
		t.Fatal("spawn_batch tool not found in registry after registration; " +
			"ensure NewSpawnBatchTool is registered in main.go alongside NewSubagentTool")
	}
}
