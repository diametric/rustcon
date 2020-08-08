package stats

import (
	"fmt"
	"strings"
	"sync"

	"github.com/d5/tengo"
)

var globalsValue map[string]tengo.Object = make(map[string]tengo.Object)
var globalsLock sync.Mutex

// String returns a string representation
func (o *TengoGlobals) String() string {
	globalsLock.Lock()
	defer globalsLock.Unlock()

	var elements []string
	for _, e := range globalsValue {
		elements = append(elements, e.String())
	}
	return fmt.Sprintf("[%s]", strings.Join(elements, ", "))
}

// TypeName returns the type name
func (o *TengoGlobals) TypeName() string {
	return "globals-map"
}

// IndexGet returns the value for the given key.
func (o *TengoGlobals) IndexGet(index tengo.Object) (res tengo.Object, err error) {
	globalsLock.Lock()
	defer globalsLock.Unlock()

	strIdx, ok := tengo.ToString(index)
	if !ok {
		err = tengo.ErrInvalidIndexType
		return
	}
	res, ok = globalsValue[strIdx]
	if !ok {
		res = tengo.UndefinedValue
	}
	return
}

// IndexSet sets the value for the given key.
func (o *TengoGlobals) IndexSet(index, value tengo.Object) (err error) {
	globalsLock.Lock()
	defer globalsLock.Unlock()

	strIdx, ok := tengo.ToString(index)
	if !ok {
		err = tengo.ErrInvalidIndexType
		return
	}
	globalsValue[strIdx] = value
	return nil
}
