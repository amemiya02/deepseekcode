package main

import "fmt"

func main() {
	cfg := LoadConfig()
	svc := NewService(cfg)
	result, err := svc.Process("input-data")
	if err != nil {
		fmt.Printf("error: %v\n", err)
		return
	}
	fmt.Printf("result: %s\n", result)
}
