package simple

// A and B are two distinct types that both have a method named String,
// used by TestParserMethodReceiverDisambiguation to verify that the parser
// assigns distinct NodeIDs to same-named methods on different receiver types.

type A struct{}
type B struct{}

// String on A.
func (a *A) String() string { return "A" }

// String on B.
func (b *B) String() string { return "B" }
