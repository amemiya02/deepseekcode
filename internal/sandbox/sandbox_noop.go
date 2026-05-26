//go:build !darwin && !linux

package sandbox

func defaultSandbox() Sandbox { return noopSandbox{} }

func RunSandboxedChild(args []string) error {
	return errSandboxRunUnsupported
}
