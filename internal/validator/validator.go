package validator

import (
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"
)

// Validator validates structs based on their tags
type Validator struct {
	tagName string
}

// New creates a new validator
func New() *Validator {
	return &Validator{
		tagName: "schema",
	}
}

// Validate validates a struct based on its schema tags
func (v *Validator) Validate(s interface{}) error {
	val := reflect.ValueOf(s)
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}

	if val.Kind() != reflect.Struct {
		return fmt.Errorf("expected struct, got %s", val.Kind())
	}

	typ := val.Type()
	
	for i := 0; i < val.NumField(); i++ {
		field := val.Field(i)
		structField := typ.Field(i)
		
		if !structField.IsExported() {
			continue
		}

		// Get tags
		schemaTag := structField.Tag.Get(v.tagName)
		jsonTag := structField.Tag.Get("json")
		
		// Skip fields with json:"-"
		if jsonTag == "-" {
			continue
		}

		fieldName := getFieldName(structField, jsonTag)
		
		// Validate the field
		if err := v.validateField(field, structField, schemaTag, fieldName); err != nil {
			return err
		}
	}

	return nil
}

func (v *Validator) validateField(value reflect.Value, field reflect.StructField, tag string, fieldName string) error {
	if tag == "" {
		return nil
	}

	// Handle zero values for required fields
	if strings.Contains(tag, "required") && isZeroValue(value) {
		return fmt.Errorf("field '%s' is required", fieldName)
	}

	// Skip validation for zero values if not required
	if isZeroValue(value) && !strings.Contains(tag, "required") {
		return nil
	}

	parts := strings.Split(tag, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		
		if err := v.validateTag(value, field, part, fieldName); err != nil {
			return err
		}
	}

	return nil
}

func (v *Validator) validateTag(value reflect.Value, field reflect.StructField, tag string, fieldName string) error {
	// Handle enum validation
	if strings.HasPrefix(tag, "enum:") {
		return v.validateEnum(value, tag[5:], fieldName)
	}

	// Handle min/max validation
	if strings.HasPrefix(tag, "min:") {
		return v.validateMin(value, tag[4:], fieldName)
	}

	if strings.HasPrefix(tag, "max:") {
		return v.validateMax(value, tag[4:], fieldName)
	}

	// Handle pattern validation
	if strings.HasPrefix(tag, "pattern:") {
		return v.validatePattern(value, tag[8:], fieldName)
	}

	// Handle format validation
	if strings.HasPrefix(tag, "format:") {
		return v.validateFormat(value, tag[7:], fieldName)
	}

	return nil
}

func (v *Validator) validateEnum(value reflect.Value, enumValues string, fieldName string) error {
	allowedValues := strings.Split(enumValues, "|")
	currentValue := fmt.Sprintf("%v", value.Interface())

	for _, allowed := range allowedValues {
		if currentValue == allowed {
			return nil
		}
	}

	return fmt.Errorf("field '%s' must be one of: %s", fieldName, strings.Join(allowedValues, ", "))
}

func (v *Validator) validateMin(value reflect.Value, minStr string, fieldName string) error {
	switch value.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		min, err := strconv.ParseInt(minStr, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid min value for field '%s': %s", fieldName, minStr)
		}
		if value.Int() < min {
			return fmt.Errorf("field '%s' must be at least %d", fieldName, min)
		}
	case reflect.Float32, reflect.Float64:
		min, err := strconv.ParseFloat(minStr, 64)
		if err != nil {
			return fmt.Errorf("invalid min value for field '%s': %s", fieldName, minStr)
		}
		if value.Float() < min {
			return fmt.Errorf("field '%s' must be at least %f", fieldName, min)
		}
	case reflect.String:
		minLen, err := strconv.Atoi(minStr)
		if err != nil {
			return fmt.Errorf("invalid min length for field '%s': %s", fieldName, minStr)
		}
		if len(value.String()) < minLen {
			return fmt.Errorf("field '%s' must be at least %d characters", fieldName, minLen)
		}
	}
	return nil
}

func (v *Validator) validateMax(value reflect.Value, maxStr string, fieldName string) error {
	switch value.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		max, err := strconv.ParseInt(maxStr, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid max value for field '%s': %s", fieldName, maxStr)
		}
		if value.Int() > max {
			return fmt.Errorf("field '%s' must be at most %d", fieldName, max)
		}
	case reflect.Float32, reflect.Float64:
		max, err := strconv.ParseFloat(maxStr, 64)
		if err != nil {
			return fmt.Errorf("invalid max value for field '%s': %s", fieldName, maxStr)
		}
		if value.Float() > max {
			return fmt.Errorf("field '%s' must be at most %f", fieldName, max)
		}
	case reflect.String:
		maxLen, err := strconv.Atoi(maxStr)
		if err != nil {
			return fmt.Errorf("invalid max length for field '%s': %s", fieldName, maxStr)
		}
		if len(value.String()) > maxLen {
			return fmt.Errorf("field '%s' must be at most %d characters", fieldName, maxLen)
		}
	}
	return nil
}

func (v *Validator) validatePattern(value reflect.Value, pattern string, fieldName string) error {
	if value.Kind() != reflect.String {
		return nil
	}

	re, err := regexp.Compile(pattern)
	if err != nil {
		return fmt.Errorf("invalid pattern for field '%s': %s", fieldName, pattern)
	}

	if !re.MatchString(value.String()) {
		return fmt.Errorf("field '%s' does not match pattern: %s", fieldName, pattern)
	}

	return nil
}

func (v *Validator) validateFormat(value reflect.Value, format string, fieldName string) error {
	if value.Kind() != reflect.String {
		return nil
	}

	str := value.String()

	switch format {
	case "email":
		emailRegex := regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)
		if !emailRegex.MatchString(str) {
			return fmt.Errorf("field '%s' must be a valid email address", fieldName)
		}
	case "url", "uri":
		urlRegex := regexp.MustCompile(`^https?://[^\s/$.?#].[^\s]*$`)
		if !urlRegex.MatchString(str) {
			return fmt.Errorf("field '%s' must be a valid URL", fieldName)
		}
	case "uuid":
		uuidRegex := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
		if !uuidRegex.MatchString(str) {
			return fmt.Errorf("field '%s' must be a valid UUID", fieldName)
		}
	}

	return nil
}

func isZeroValue(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Array, reflect.Map, reflect.Slice, reflect.String:
		return v.Len() == 0
	case reflect.Bool:
		return !v.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return v.Uint() == 0
	case reflect.Float32, reflect.Float64:
		return v.Float() == 0
	case reflect.Interface, reflect.Ptr:
		return v.IsNil()
	}
	return false
}

func getFieldName(field reflect.StructField, jsonTag string) string {
	if jsonTag == "" {
		return field.Name
	}

	parts := strings.Split(jsonTag, ",")
	name := strings.TrimSpace(parts[0])
	
	if name == "" {
		return field.Name
	}

	return name
}

// DefaultValidator is the default validator instance
var DefaultValidator = New()

// Validate validates a struct using the default validator
func Validate(s interface{}) error {
	return DefaultValidator.Validate(s)
}