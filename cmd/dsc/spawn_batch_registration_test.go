package main

import (
	"testing"

	"github.com/amemiya02/deepseekcode/internal/agent"
	"github.com/amemiya02/deepseekcode/internal/tools"
	"github.com/amemiya02/deepseekcode/internal/worktree"
)

// TestSpawnBatchRegisteredViaProductionHelper is an integration-level check
// that the production registration path — registerSpawnerTools — includes
// spawn_batch. It calls the same helper that runTUI and runOneShot call, so
// deleting the reg.Register(tools.NewSpawnBatchTool(...)) line from
// registerSpawnerTools will make this test fail.
//
// The previous unit-level test (which registered the tool directly) was
// insufficient: it did not exercise the production path and would pass even
// if both Registration calls were removed from main.go.
func TestSpawnBatchRegisteredViaProductionHelper(t *testing.T) {
	reg := tools.New()
	spawner := &agent.LoopSpawner{} // nil fields are fine; registration only needs the value
	wtMgr := worktree.NewManager(".")

	registerSpawnerTools(reg, spawner, wtMgr)

	for _, name := range []string{"task", "spawn_batch", "worktree"} {
		if _, ok := reg.Get(name); !ok {
			t.Errorf("registerSpawnerTools: tool %q not found in registry; "+
				"ensure it is registered in registerSpawnerTools (main.go)", name)
		}
	}
}
