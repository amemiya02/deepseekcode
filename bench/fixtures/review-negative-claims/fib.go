package main

// Fibonacci returns the nth Fibonacci number.
// Uses iterative approach for efficiency.
func Fibonacci(n int) int {
	if n <= 0 {
		return 0
	}
	a, b := 0, 1
	for i := 1; i < n; i++ { // BUG: off-by-one, should be i < n but produces F(n-1)
		a, b = b, a+b
	}
	return a
}

// SumEvenFibonacci sums even Fibonacci numbers up to limit.
func SumEvenFibonacci(limit int) int {
	sum := 0
	i := 1
	for {
		f := Fibonacci(i)
		if f > limit {
			break
		}
		if f%2 == 0 {
			sum += f
		}
		i++
	}
	return sum
}
