package inject

import (
	"reflect"
	"fmt"
)

type InjectedFunction struct {
	backing func() ([]interface{})
	errorIdx *int
}

func (i InjectedFunction) ActualizeNoError() func() func(to ...interface{}) {
	actualized := i.Actualize()
	return func() (func(to ...interface{})) {
		setter := actualized()
		return func(to ...interface{}) {
			err := setter(to...)
			if err != nil {
				panic(err)
			}
		}
	}
}

func (i InjectedFunction) Actualize() func() (func(to ...interface{}) error) {
	return func() (func (...interface{}) error) {
		out := i.backing()
		return func(to ...interface{}) (err error) {
			if len(out) != len(to) {
				panic("must get all return values")
			}

			for outI, destI := 0, 0; outI < len(out); {
				if i.errorIdx != nil && outI == *i.errorIdx {
					err, _ = out[outI].(error)
				}  else {
					destPtr := reflect.ValueOf(to[destI])
					destI++
					destType := destPtr.Type()
					if destType.Kind() != reflect.Ptr {
						panic("cannot set to non-pointer")
					}
					outVal := reflect.ValueOf(out[i])
					if !outVal.Type().AssignableTo(destType.Elem()) {
						panic(fmt.Sprintf("invalid type %s assign to %s", outVal.Type().String(), destType.String()))
					}
					destPtr.Elem().Set(outVal)
				}
				outI++
			}
			return
		}
	}
}
