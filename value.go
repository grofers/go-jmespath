package jmespath

import (
	"log"
	"reflect"
	"strings"
	"encoding/json"
	conv "github.com/cstockton/go-conv"
)

type Value struct {
	is_expression	bool

	// For non-expressions
	resolved		interface{}
	resolvedval		reflect.Value
	
	// For expressions
	expression		expRef
}

func AsValue(inp interface{}) *Value {
	expression, ok := inp.(expRef)
	if ok {
		return &Value {
			is_expression: true,
			expression: expression,
		}
	}

	val, _ := stripPtrs(reflect.ValueOf(inp))

	var resolved interface{}
	if val.IsValid() {
		resolved = val.Interface()
	} else {
		resolved = nil
	}

	return &Value {
		resolved: resolved,
		resolvedval: val,
		is_expression: false,
	}
}

func (v *Value) IsString() bool {
	return v.resolvedval.Kind() == reflect.String
}

func (v *Value) IsBool() bool {
	return v.resolvedval.Kind() == reflect.Bool
}

func (v *Value) IsFloat() bool {
	return v.resolvedval.Kind() == reflect.Float32 ||
		v.resolvedval.Kind() == reflect.Float64
}

func (v *Value) IsInteger() bool {
	return v.resolvedval.Kind() == reflect.Int ||
		v.resolvedval.Kind() == reflect.Int8 ||
		v.resolvedval.Kind() == reflect.Int16 ||
		v.resolvedval.Kind() == reflect.Int32 ||
		v.resolvedval.Kind() == reflect.Int64 ||
		v.resolvedval.Kind() == reflect.Uint ||
		v.resolvedval.Kind() == reflect.Uint8 ||
		v.resolvedval.Kind() == reflect.Uint16 ||
		v.resolvedval.Kind() == reflect.Uint32 ||
		v.resolvedval.Kind() == reflect.Uint64
}

func (v *Value) IsNumber() bool {
	if _, ok := v.resolved.(json.Number); ok {
		return true
	}
	return v.IsInteger() || v.IsFloat()
}

func (v *Value) IsNil() bool {
	return !v.resolvedval.IsValid()
}

func (v *Value) String() (out string) {
	out, _ = conv.String(v.resolved)
	return
}

func (v *Value) Integer() (out int64) {
	out, _ = conv.Int64(v.resolved)
	return
}

func (v *Value) Float() (out float64) {
	out, _ = conv.Float64(v.resolved)
	return
}

func (v *Value) Bool() (out bool) {
	out, _ = conv.Bool(v.resolved)
	return
}

func (v *Value) Len() int {
	switch v.resolvedval.Kind() {
	case reflect.Array, reflect.Chan, reflect.Map, reflect.Slice:
		return v.resolvedval.Len()
	case reflect.String:
		runes := []rune(v.resolvedval.String())
		return len(runes)
	default:
		log.Printf("Value.Len() not available for type: %s\n", v.resolvedval.Kind().String())
		return 0
	}
}

func (v *Value) Slice(i, j int) *Value {
	switch v.resolvedval.Kind() {
	case reflect.Array, reflect.Slice:
		return AsValue(v.resolvedval.Slice(i, j).Interface())
	case reflect.String:
		runes := []rune(v.resolvedval.String())
		return AsValue(string(runes[i:j]))
	default:
		log.Printf("Value.Slice() not available for type: %s\n", v.resolvedval.Kind().String())
		return AsValue([]int{})
	}
}

func (v *Value) Index(i int) *Value {
	switch v.resolvedval.Kind() {
	case reflect.Array, reflect.Slice:
		if i >= v.Len() {
			return AsValue(nil)
		}
		return AsValue(v.resolvedval.Index(i).Interface())
	case reflect.String:
		s := v.resolvedval.String()
		runes := []rune(s)
		if i < len(runes) {
			return AsValue(string(runes[i]))
		}
		return AsValue("")
	default:
		log.Printf("Value.Slice() not available for type: %s\n", v.resolvedval.Kind().String())
		return AsValue([]int{})
	}
}

func (v *Value) Contains(other *Value) bool {
	switch v.resolvedval.Kind() {
	case reflect.Struct:
		fieldValue := v.resolvedval.FieldByName(other.String())
		return fieldValue.IsValid()
	case reflect.Map:
		var mapValue reflect.Value
		switch other.Interface().(type) {
		case int:
			mapValue = v.resolvedval.MapIndex(other.resolvedval)
		case string:
			mapValue = v.resolvedval.MapIndex(other.resolvedval)
		default:
			log.Printf("Value.Contains() does not support lookup type '%s'\n", other.resolvedval.Kind().String())
			return false
		}

		return mapValue.IsValid()
	case reflect.String:
		return strings.Contains(v.resolvedval.String(), other.String())

	case reflect.Slice, reflect.Array:
		for i := 0; i < v.resolvedval.Len(); i++ {
			item := v.resolvedval.Index(i)
			if other.Interface() == item.Interface() {
				return true
			}
		}
		return false

	default:
		log.Printf("Value.Contains() not available for type: %s\n", v.resolvedval.Kind().String())
		return false
	}
}

func (v *Value) CanSlice() bool {
	switch v.resolvedval.Kind() {
	case reflect.Array, reflect.Slice, reflect.String:
		return true
	}
	return false
}

func (v *Value) Interface() interface{} {
	return v.resolved
}

func (v *Value) Reflect() reflect.Value {
	return v.resolvedval
}

func (v *Value) IsExpression() bool {
	return v.is_expression
}
