package inject

import (
	"reflect"
	"fmt"
)

func NewDependencyInjector() (out *DependencyInjector) {
	out = new(DependencyInjector)
	out.Singleton(out)
	return
}

type injectorFunc func(t reflect.Type) interface{}

type DependencyInjector struct {
	parent    *DependencyInjector
	injectors map[reflect.Type]injectorFunc
}

func (dep *DependencyInjector) BindProvider(something interface{}, source injectorFunc) {
	if dep.injectors == nil {
		dep.injectors = make(map[reflect.Type]injectorFunc)
	}

	t := cleanType(reflect.TypeOf(something))
	dep.checkConflict(t)
	dep.injectors[t] = source
}

func (dep *DependencyInjector) checkConflict(t reflect.Type) {
	if _, has := dep.injectors[t]; has {
		panic("already registered type")
	}
}

func (dep *DependencyInjector) BindZero(something interface{}) {
	dep.BindProvider(something, func(t reflect.Type) interface{} {
		return reflect.New(t).Elem()
	})
}

func (dep *DependencyInjector) Singleton(value interface{}) {
	dep.BindProvider(value, func(t reflect.Type) interface{} {
		return value
	})
}

func (dep *DependencyInjector) getInjector(t reflect.Type) (f injectorFunc, has bool) {
	f, has = dep.injectors[t]
	if !has && dep.parent != nil {
		f, has = dep.parent.getInjector(t)
	}
	if !has && t.Kind() == reflect.Interface {
		for key, fVal := range dep.injectors {
			if key.Implements(t) {
				f = fVal
				has = true
				return
			}
		}
	}
	return
}

func (dep *DependencyInjector) cloneInternalState() {
	out := make(map[reflect.Type]injectorFunc, len(dep.injectors))
	for t, provider := range dep.injectors {
		out[t] = provider
	}
	dep.injectors = out
}

func (dep DependencyInjector) Provider(t reflect.Type) (f func(to interface{}), has bool) {
	t = cleanType(t)
	source, has := dep.getInjector(t)
	if !has {
		return
	}
	var countPtr func(reflect.Type) int
	countPtr = func(val reflect.Type) int {
		if val.Kind() == reflect.Ptr {
			return 1 + countPtr(val.Elem())
		}
		return 0
	}

	f = func (to interface{}) {
		value := source(t)

		//to's pointer count should be one higher than the value's pointer count
		valueReflect, toReflect := reflect.ValueOf(value), reflect.ValueOf(to)
		valueT, toT := valueReflect.Type(), toReflect.Type()
		valuePtr, toPtr := countPtr(valueT), countPtr(toT)
		for valuePtr + 1 != toPtr {
			delta := valuePtr - toPtr
			if delta < 0 {
				valueT = reflect.PtrTo(valueT)
				ptrTo := reflect.New(valueT)
				ptrTo.Elem().Set(valueReflect)
				valueReflect = ptrTo
				valuePtr++
			} else if delta > 0 {
				valueT = valueT.Elem()
				valueReflect = valueReflect.Elem()
				valuePtr--
			}
		}

		if !valueT.AssignableTo(toT.Elem()) {
			panic(fmt.Sprintf("cannot assign from %s to %s", valueT.String(), toT.String()))
		}

		toReflect.Elem().Set(valueReflect)
	}
	return
}

func (dep DependencyInjector) Inject(ptr interface{}) {
	if ptr == nil {
		panic("nil pointer passed")
	}

	p := reflect.ValueOf(ptr)
	if p.Kind() != reflect.Ptr {
		panic("cannot inject to immutable copy")
	}

	elemType := p.Type().Elem()
	if elemType.Kind() != reflect.Struct {
		panic("cannot set to non-struct")
	}
	elem := p.Elem()
	for i := 0; i < elemType.NumField(); i++ {
		field := elem.Field(i)
		if !field.CanInterface() {
			continue
		}
		provider, has := dep.Provider(field.Type())
		if !has {
			continue
		}

		provider(field.Addr().Interface())
	}
}

func (dep DependencyInjector) PreInjectAll(f interface{}) (fOut InjectedFunction) {
	fValue := reflect.ValueOf(f)
	if fValue .Kind() != reflect.Func {
		panic("must pass function")
	}
	fType := fValue.Type()
	inProviders := make([]func (interface{}), fType.NumIn())
	for i := 0; i < fType.NumIn(); i++ {
		inProviders[i], _ = dep.Provider(fType.In(i))
	}
	caller := func() (out []interface{}) {
		inValues := make([]reflect.Value, len(inProviders))
		for i := range inValues {
			ptr := reflect.New(fType.In(i)).Interface()
			inProviders[i](ptr)
			inValues[i] = reflect.ValueOf(ptr).Elem()
		}
		returns := fValue.Call(inValues)
		out = make([]interface{}, len(returns))
		for i := range returns {
			out[i] = returns[i].Interface()
		}
		return
	}
	var errIndex *int
	for i := 0; i < fType.NumOut(); i++ {
		if fType.Out(i).Implements(reflect.TypeOf((*error)(nil)).Elem()) {
			errIndex = new(int)
			*errIndex = i
			break
		}
	}
	fOut.backing = caller
	fOut.errorIdx = errIndex
	return
}

func (dep DependencyInjector) ChildInjector() (out *DependencyInjector) {
	(&dep).cloneInternalState()
	out = new(DependencyInjector)
	out.parent = &dep
	return
}
