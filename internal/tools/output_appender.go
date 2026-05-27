package tools

// OutputAppender is an interface for appending output to a buffer.
// Implemented by the agent's Job type.
type OutputAppender interface {
	AppendOutput(p []byte)
}
