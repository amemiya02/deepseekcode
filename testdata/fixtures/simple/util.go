package simple

// Add adds two ints.
func Add(a, b int) int { return a + b }

// Multiply multiplies two ints.
func Multiply(a, b int) int { return a * b }

// Calculator is a type.
type Calculator struct{}

// Doer is an interface.
type Doer interface {
	Do() error
}
