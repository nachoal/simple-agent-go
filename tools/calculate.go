package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/nachoal/simple-agent-go/tools/base"
)

// CalculateParams now uses generic input like Ruby
// The input string is the expression directly
type CalculateParams = base.GenericParams

// CalculateTool evaluates mathematical expressions
type CalculateTool struct {
	base.BaseTool
}

// Parameters returns the parameters struct
func (t *CalculateTool) Parameters() interface{} {
	return &base.GenericParams{}
}

// Execute evaluates a mathematical expression
func (t *CalculateTool) Execute(ctx context.Context, params json.RawMessage) (string, error) {
	var args base.GenericParams
	if err := json.Unmarshal(params, &args); err != nil {
		return "", NewToolError("INVALID_PARAMS", "Failed to parse parameters").
			WithDetail("error", err.Error())
	}

	// In Ruby style, the input is the expression directly
	expr := strings.TrimSpace(args.Input)
	if expr == "" {
		return "", NewToolError("EMPTY_EXPRESSION", "Expression cannot be empty")
	}

	// For now, implement a simple calculator
	// In production, use a proper expression evaluator like govaluate
	result, err := t.evaluateSimple(expr)
	if err != nil {
		return "", NewToolError("EVALUATION_ERROR", "Failed to evaluate expression").
			WithDetail("error", err.Error()).
			WithDetail("expression", expr)
	}

	return fmt.Sprintf("%s = %v", expr, result), nil
}

// evaluateSimple is a basic expression evaluator
// In production, replace with a proper expression parsing library
func (t *CalculateTool) evaluateSimple(expr string) (float64, error) {
	// Remove spaces
	expr = strings.ReplaceAll(expr, " ", "")

	// Handle basic operations
	// This is a simplified implementation
	// Real implementation should use proper expression parsing

	// Try to parse as a simple number first
	if val, err := strconv.ParseFloat(expr, 64); err == nil {
		return val, nil
	}

	// Handle basic binary operations
	operators := []string{"+", "-", "*", "/", "^"}
	for _, op := range operators {
		parts := strings.SplitN(expr, op, 2)
		if len(parts) == 2 {
			left, err1 := t.evaluateSimple(parts[0])
			right, err2 := t.evaluateSimple(parts[1])

			if err1 != nil || err2 != nil {
				continue
			}

			switch op {
			case "+":
				return left + right, nil
			case "-":
				return left - right, nil
			case "*":
				return left * right, nil
			case "/":
				if right == 0 {
					return 0, fmt.Errorf("division by zero")
				}
				return left / right, nil
			case "^":
				return math.Pow(left, right), nil
			}
		}
	}

	// Handle parentheses
	if strings.HasPrefix(expr, "(") && strings.HasSuffix(expr, ")") {
		return t.evaluateSimple(expr[1 : len(expr)-1])
	}

	// Handle basic functions
	functions := map[string]func(float64) float64{
		"sqrt": math.Sqrt,
		"sin":  math.Sin,
		"cos":  math.Cos,
		"tan":  math.Tan,
		"log":  math.Log10,
		"ln":   math.Log,
		"abs":  math.Abs,
	}

	for fname, fn := range functions {
		if strings.HasPrefix(expr, fname+"(") && strings.HasSuffix(expr, ")") {
			inner := expr[len(fname)+1 : len(expr)-1]
			val, err := t.evaluateSimple(inner)
			if err != nil {
				return 0, err
			}
			return fn(val), nil
		}
	}

	// Handle constants
	constants := map[string]float64{
		"pi": math.Pi,
		"e":  math.E,
		"PI": math.Pi,
		"E":  math.E,
	}

	if val, ok := constants[expr]; ok {
		return val, nil
	}

	return 0, fmt.Errorf("unable to evaluate expression: %s", expr)
}
