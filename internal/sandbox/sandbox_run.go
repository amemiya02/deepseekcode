package sandbox

import "errors"

var errSandboxRunUnsupported = errors.New("__sandbox_run is only supported on linux")
