package stats

import (
	"strings"

	"github.com/d5/tengo/v2"
	"go.uber.org/zap"
)

// CanCall returns true since we're a function type.
func (o *TengoLogger) CanCall() bool {
	return true
}

// TypeName returns the logger name
func (o *TengoLogger) TypeName() string {
	return "logger"
}

// String returns the logger name
func (o *TengoLogger) String() string {
	return "logger"
}

// Call provides the logger functionality.
func (o *TengoLogger) Call(args ...tengo.Object) (ret tengo.Object, err error) {
	if len(args) != 2 {
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

	s2, ok := tengo.ToString(args[1])
	if !ok {
		return nil, tengo.ErrInvalidArgumentType{
			Name:     "first",
			Expected: "string",
			Found:    args[0].TypeName(),
		}
	}

	switch strings.ToLower(s1) {
	case "warn":
		zap.S().Warn(s2)
	case "warning":
		zap.S().Warn(s2)
	case "info":
		zap.S().Info(s2)
	case "error":
		zap.S().Error(s2)
	case "debug":
		zap.S().Debug(s2)
	default:
		return nil, tengo.ErrInvalidArgumentType{
			Name:     "first",
			Expected: "string (one of warn, warning, info, error, or debug)",
			Found:    s1,
		}
	}

	return tengo.UndefinedValue, nil
}
