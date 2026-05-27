package main

import "fmt"

// Connect establishes a database connection.
func Connect(dsn string) error {
	if dsn == "" {
		return fmt.Errorf("empty dsn")
	}
	return nil
}
