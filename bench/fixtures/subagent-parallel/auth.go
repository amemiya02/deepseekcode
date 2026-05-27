package main

import "errors"

// Authenticate checks credentials.
func Authenticate(user, pass string) error {
	if user == "" {
		return errors.New("empty username")
	}
	if pass == "" {
		return errors.New("empty password")
	}
	return nil
}
