package h2h

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadTasksValidatesRequiredFields(t *testing.T) {
	dir := t.TempDir()
	good := `[{"id":"grpc-3476","repo":"https://github.com/grpc/grpc-go","commit":"abc123","prompt":"fix the bug","fail_to_pass":["TestFoo"],"test_dir":"./rls/...","turn_cap":30,"wallclock_cap_min":20}]`
	p := filepath.Join(dir, "tasks.json")
	os.WriteFile(p, []byte(good), 0o644)
	tasks, err := LoadTasks(p)
	if err != nil {
		t.Fatalf("LoadTasks: %v", err)
	}
	if len(tasks) != 1 || tasks[0].ID != "grpc-3476" || tasks[0].TurnCap != 30 {
		t.Fatalf("bad parse: %+v", tasks)
	}

	bad := `[{"id":"x","repo":"","commit":"abc","prompt":"p","fail_to_pass":[],"test_dir":"."}]`
	os.WriteFile(p, []byte(bad), 0o644)
	if _, err := LoadTasks(p); err == nil {
		t.Fatal("want validation error for empty repo + empty fail_to_pass")
	}
}