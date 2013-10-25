package xmlrpc

import (
	"fmt"
)

type errorStruct struct {
	main    error
	message string
}

func (es errorStruct) Error() string {
	return es.main.Error() + " [" + es.message + "]"
}

// Errorf2 returns an error embedding the main error with the formatted message
func Errorf2(err error, format string, a ...interface{}) error {
	return &errorStruct{main: err, message: fmt.Sprintf(format, a...)}
}

// ErrEq checks equality of the errorStructs (equality of the embedded main errors
func ErrEq(a, b error) bool {
	var maina, mainb error = a, b
	if esa, ok := a.(errorStruct); ok {
		maina = esa.main
	} else if esa, ok := a.(*errorStruct); ok {
		maina = esa.main
	}
	if esb, ok := b.(errorStruct); ok {
		mainb = esb.main
	} else if esb, ok := b.(*errorStruct); ok {
		mainb = esb.main
	}
	return maina == mainb
}
