package simple

// Run calls Add and Multiply.
func Run() int {
	x := Add(1, 2)
	y := Multiply(x, 3)
	return y
}
