package stats

import (
	"strings"

	"github.com/d5/tengo"
)

// CanCall returns true since we're a function type.
func (o *TengoTagEscape) CanCall() bool {
	return true
}

// TypeName returns the function name
func (o *TengoTagEscape) TypeName() string {
	return "tagescape"
}

// String returns the function name
func (o *TengoTagEscape) String() string {
	return "tagescape"
}

// Call provides the escape functionality.
func (o *TengoTagEscape) Call(args ...tengo.Object) (ret tengo.Object, err error) {
	if len(args) != 1 {
		return nil, tengo.ErrWrongNumArguments
	}

	s1, ok := tengo.ToString(args[0])
	if !ok {
		return nil, tengo.ErrInvalidArgumentType{
			Name:     "first",
			Expected: "string",
			Found:    args[0].TypeName(),
		}
	}

	s1 = strings.ReplaceAll(s1, ",", "\\,")
	s1 = strings.ReplaceAll(s1, " ", "\\ ")
	s1 = strings.ReplaceAll(s1, "=", "\\=")

	return &tengo.String{Value: s1}, nil
}

// CanCall returns true since we're a function type.
func (o *TengoFieldEscape) CanCall() bool {
	return true
}

// TypeName returns the function name
func (o *TengoFieldEscape) TypeName() string {
	return "fieldescape"
}

// String returns the function name
func (o *TengoFieldEscape) String() string {
	return "fieldescape"
}

// Call provides the field escape functionality.
func (o *TengoFieldEscape) Call(args ...tengo.Object) (ret tengo.Object, err error) {
	if len(args) != 1 {
		return nil, tengo.ErrWrongNumArguments
	}

	s1, ok := tengo.ToString(args[0])
	if !ok {
		return nil, tengo.ErrInvalidArgumentType{
			Name:     "first",
			Expected: "string",
			Found:    args[0].TypeName(),
		}
	}

	s1 = strings.ReplaceAll(s1, "\\", "\\\\")

	return &tengo.String{Value: s1}, nil
}
