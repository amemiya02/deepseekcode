package tools

import (
	"context"
	"os/exec"
	"strings"

	"github.com/amemiya02/deepseekcode/internal/sandbox"
)

type fakeSandbox struct {
	name      string
	available bool
	denied    string
	wrapped   bool
}

func (f *fakeSandbox) Name() string {
	if f.name == "" {
		return "fake"
	}
	return f.name
}

func (f *fakeSandbox) Available() bool { return f.available }

func (f *fakeSandbox) Wrap(ctx context.Context, p sandbox.Profile, cmd *exec.Cmd) error {
	f.wrapped = true
	return nil
}

func (f *fakeSandbox) WasDenied(stderr string) bool {
	return f.denied != "" && strings.Contains(stderr, f.denied)
}
