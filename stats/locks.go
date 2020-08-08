package stats

import (
	"sync"

	"github.com/d5/tengo"
)

var tengoLock sync.Mutex

// CanCall returns true since we're a function type.
func (o *TengoLock) CanCall() bool {
	return true
}

// TypeName returns the name lock
func (o *TengoLock) TypeName() string {
	return "lock"
}

// String returns the string lock
func (o *TengoLock) String() string {
	return "lock"
}

// Call provides the lock functionality.
func (o *TengoLock) Call(args ...tengo.Object) (ret tengo.Object, err error) {
	if len(args) > 0 {
		return nil, tengo.ErrWrongNumArguments
	}

	tengoLock.Lock()

	return tengo.UndefinedValue, nil
}

// CanCall returns true since we're a function type.
func (o *TengoUnlock) CanCall() bool {
	return true
}

// TypeName returns the name unlock
func (o *TengoUnlock) TypeName() string {
	return "unlock"
}

// String returns the string unlock
func (o *TengoUnlock) String() string {
	return "unlock"
}

// Call provides the lock functionality.
func (o *TengoUnlock) Call(args ...tengo.Object) (ret tengo.Object, err error) {
	if len(args) > 0 {
		return nil, tengo.ErrWrongNumArguments
	}

	tengoLock.Unlock()

	return tengo.UndefinedValue, nil
}
