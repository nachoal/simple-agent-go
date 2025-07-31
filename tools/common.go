package tools

import "github.com/nachoal/simple-agent-go/internal/validator"

// Validate is a convenience function that validates a struct using the default validator
func Validate(s interface{}) error {
	return validator.Validate(s)
}