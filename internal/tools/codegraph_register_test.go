package tools_test

import (
	"testing"

	"github.com/amemiya02/deepseekcode/internal/codegraph"
	"github.com/amemiya02/deepseekcode/internal/tools"
)

func TestRegisterCodegraphTools(t *testing.T) {
	idx := codegraph.NewIndex("github.com/amemiya02/deepseekcode/testdata/fixtures/simple")
	_ = idx.Rebuild(toolFixtureDir(t))

	reg := tools.New()
	tools.RegisterCodegraphTools(reg, idx)

	want := []string{"codegraph_search", "codegraph_callers", "codegraph_callees", "codegraph_impact"}
	all := reg.All()
	registered := map[string]bool{}
	for _, tool := range all {
		registered[tool.Name()] = true
	}
	for _, name := range want {
		if !registered[name] {
			t.Errorf("tool %q not registered; registered: %v", name, registered)
		}
	}
}
