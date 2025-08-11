package base

// GenericParams is the universal parameter structure for all tools
// This matches Ruby's approach of using a single "input" parameter
type GenericParams struct {
	Input string `json:"input" schema:"required" description:"Input for the tool"`
}
