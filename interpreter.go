package jmespath

import (
	"errors"
	"reflect"
	"unicode"
	"unicode/utf8"
	conv "github.com/cstockton/go-conv"
	reflections "github.com/oleiade/reflections"
)

/* This is a tree based interpreter.  It walks the AST and directly
   interprets the AST to search through a JSON document.
*/

type treeInterpreter struct {
	fCall *functionCaller
}

func newInterpreter() *treeInterpreter {
	interpreter := treeInterpreter{}
	interpreter.fCall = newFunctionCaller()
	return &interpreter
}

type expRef struct {
	ref ASTNode
}

// Execute takes an ASTNode and input data and interprets the AST directly.
// It will produce the result of applying the JMESPath expression associated
// with the ASTNode to the input data "value".
func (intr *treeInterpreter) Execute(node ASTNode, value interface{}) (interface{}, error) {
	rootValue := value
	return intr.execute(node, value, rootValue)
}

func (intr *treeInterpreter) execute(node ASTNode, value interface{}, rootValue interface{}) (interface{}, error) {
	switch node.nodeType {
	case ASTComparator:
		left, err := intr.execute(node.children[0], value, rootValue)
		if err != nil {
			return nil, err
		}
		right, err := intr.execute(node.children[1], value, rootValue)
		if err != nil {
			return nil, err
		}
		switch node.value {
		case tEQ:
			return objsEqual(left, right), nil
		case tNE:
			return !objsEqual(left, right), nil
		}
		leftNum, err := conv.Float64(left)
		if err != nil {
			return nil, err
		}
		rightNum, err := conv.Float64(right)
		if err != nil {
			return nil, err
		}
		switch node.value {
		case tGT:
			return leftNum > rightNum, nil
		case tGTE:
			return leftNum >= rightNum, nil
		case tLT:
			return leftNum < rightNum, nil
		case tLTE:
			return leftNum <= rightNum, nil
		}
	case ASTExpRef:
		return expRef{ref: node.children[0]}, nil
	case ASTFunctionExpression:
		resolvedArgs := []interface{}{}
		for _, arg := range node.children {
			current, err := intr.execute(arg, value, rootValue)
			if err != nil {
				return nil, err
			}
			resolvedArgs = append(resolvedArgs, current)
		}
		return intr.fCall.CallFunction(node.value.(string), resolvedArgs, intr)
	case ASTField:
		return intr.fieldFromStructOrMap(node.value.(string), value)
	case ASTFilterProjection:
		left, err := intr.execute(node.children[0], value, rootValue)
		if err != nil {
			return nil, nil
		}
		sliceType, ok := left.([]interface{})
		if !ok {
			if isSliceType(left) {
				return intr.filterProjectionWithReflection(node, left)
			}
			return nil, nil
		}
		compareNode := node.children[2]
		collected := []interface{}{}
		for _, element := range sliceType {
			result, err := intr.execute(compareNode, element, rootValue)
			if err != nil {
				return nil, err
			}
			if !isFalse(result) {
				current, err := intr.execute(node.children[1], element, rootValue)
				if err != nil {
					return nil, err
				}
				if current != nil {
					collected = append(collected, current)
				}
			}
		}
		return collected, nil
	case ASTFlatten:
		left, err := intr.execute(node.children[0], value, rootValue)
		if err != nil {
			return nil, nil
		}
		sliceType, ok := left.([]interface{})
		if !ok {
			// If we can't type convert to []interface{}, there's
			// a chance this could still work via reflection if we're
			// dealing with user provided types.
			if isSliceType(left) {
				return intr.flattenWithReflection(left)
			}
			return nil, nil
		}
		flattened := []interface{}{}
		for _, element := range sliceType {
			if elementSlice, ok := element.([]interface{}); ok {
				flattened = append(flattened, elementSlice...)
			} else if isSliceType(element) {
				reflectFlat := []interface{}{}
				v := reflect.ValueOf(element)
				for i := 0; i < v.Len(); i++ {
					reflectFlat = append(reflectFlat, v.Index(i).Interface())
				}
				flattened = append(flattened, reflectFlat...)
			} else {
				flattened = append(flattened, element)
			}
		}
		return flattened, nil
	case ASTIdentity, ASTCurrentNode:
		return value, nil
	case ASTRootNode:
		return rootValue, nil
	case ASTIndex:
		if sliceType, ok := value.([]interface{}); ok {
			index := node.value.(int)
			if index < 0 {
				index += len(sliceType)
			}
			if index < len(sliceType) && index >= 0 {
				return sliceType[index], nil
			}
			return nil, nil
		}
		// Otherwise try via reflection.
		rv := reflect.ValueOf(value)
		if rv.Kind() == reflect.Slice {
			index := node.value.(int)
			if index < 0 {
				index += rv.Len()
			}
			if index < rv.Len() && index >= 0 {
				v := rv.Index(index)
				return v.Interface(), nil
			}
		}
		return nil, nil
	case ASTKeyValPair:
		return intr.execute(node.children[0], value, rootValue)
	case ASTLiteral:
		return node.value, nil
	case ASTMultiSelectHash:
		if value == nil {
			return nil, nil
		}
		collected := make(map[string]interface{})
		for _, child := range node.children {
			current, err := intr.execute(child, value, rootValue)
			if err != nil {
				return nil, err
			}
			key := child.value.(string)
			collected[key] = current
		}
		return collected, nil
	case ASTMultiSelectList:
		if value == nil {
			return nil, nil
		}
		collected := []interface{}{}
		for _, child := range node.children {
			current, err := intr.execute(child, value, rootValue)
			if err != nil {
				return nil, err
			}
			collected = append(collected, current)
		}
		return collected, nil
	case ASTOrExpression:
		matched, err := intr.execute(node.children[0], value, rootValue)
		if err != nil {
			return nil, err
		}
		if isFalse(matched) {
			matched, err = intr.execute(node.children[1], value, rootValue)
			if err != nil {
				return nil, err
			}
		}
		return matched, nil
	case ASTAndExpression:
		matched, err := intr.execute(node.children[0], value, rootValue)
		if err != nil {
			return nil, err
		}
		if isFalse(matched) {
			return matched, nil
		}
		return intr.execute(node.children[1], value, rootValue)
	case ASTNotExpression:
		matched, err := intr.execute(node.children[0], value, rootValue)
		if err != nil {
			return nil, err
		}
		if isFalse(matched) {
			return true, nil
		}
		return false, nil
	case ASTPipe:
		result := value
		var err error
		for _, child := range node.children {
			result, err = intr.execute(child, result, rootValue)
			if err != nil {
				return nil, err
			}
		}
		return result, nil
	case ASTProjection:
		left, err := intr.execute(node.children[0], value, rootValue)
		if err != nil {
			return nil, err
		}
		sliceType, ok := left.([]interface{})
		if !ok {
			if isSliceType(left) {
				return intr.projectWithReflection(node, left)
			}
			return nil, nil
		}
		collected := []interface{}{}
		var current interface{}
		for _, element := range sliceType {
			current, err = intr.execute(node.children[1], element, rootValue)
			if err != nil {
				return nil, err
			}
			if current != nil {
				collected = append(collected, current)
			}
		}
		return collected, nil
	case ASTSubexpression, ASTIndexExpression:
		left, err := intr.execute(node.children[0], value, rootValue)
		if err != nil {
			return nil, err
		}
		return intr.execute(node.children[1], left, rootValue)
	case ASTSlice:
		sliceType, ok := value.([]interface{})
		if !ok {
			if isSliceType(value) {
				return intr.sliceWithReflection(node, value)
			}
			return nil, nil
		}
		parts := node.value.([]*int)
		sliceParams := make([]sliceParam, 3)
		for i, part := range parts {
			if part != nil {
				sliceParams[i].Specified = true
				sliceParams[i].N = *part
			}
		}
		return slice(sliceType, sliceParams)
	case ASTValueProjection:
		left, err := intr.execute(node.children[0], value, rootValue)
		if err != nil {
			return nil, nil
		}
		mapType, ok := left.(map[string]interface{})
		if !ok {
			return nil, nil
		}
		values := make([]interface{}, len(mapType))
		for _, value := range mapType {
			values = append(values, value)
		}
		collected := []interface{}{}
		for _, element := range values {
			current, err := intr.execute(node.children[1], element, rootValue)
			if err != nil {
				return nil, err
			}
			if current != nil {
				collected = append(collected, current)
			}
		}
		return collected, nil
	}
	return nil, errors.New("Unknown AST node: " + node.nodeType.String())
}

func fieldNameFromStructTag(key string, value interface{}) string {
	// TODO: This is not efficient. tag_map must be put in a global cache
	tag_map, err := reflections.TagMap(value, "json")
	fieldName := ""
	if err == nil {
		ok := false
		fieldName, ok = tag_map[key]
		if !ok {
			fieldName = ""
		}
	}
	if fieldName == "" {
		first, n := utf8.DecodeRuneInString(key)
		fieldName = string(unicode.ToUpper(first)) + key[n:]
	}
	return fieldName
}

func (intr *treeInterpreter) fieldFromStructOrMap(key string, value interface{}) (interface{}, error) {
	var err error
	rv := reflect.ValueOf(value)
	rv, err = stripPtrs(rv)
	if err != nil {
		return nil, nil
	}

	if rv.Kind() == reflect.Struct {
		v := rv.FieldByName(fieldNameFromStructTag(key, value))
		if !v.IsValid() {
			return nil, nil
		}
		v, err = stripPtrs(v)
		if err != nil {
			return nil, nil
		}
		return v.Interface(), nil
	} else if rv.Kind() == reflect.Map {
		field := rv.MapIndex(reflect.ValueOf(key))
		if field.IsValid() {
			return field.Interface(), nil
		} else {
			return nil, nil
		}
	}

	return nil, nil
}

func (intr *treeInterpreter) flattenWithReflection(value interface{}) (interface{}, error) {
	v := reflect.ValueOf(value)
	flattened := []interface{}{}
	for i := 0; i < v.Len(); i++ {
		element := v.Index(i).Interface()
		if reflect.TypeOf(element).Kind() == reflect.Slice {
			// Then insert the contents of the element
			// slice into the flattened slice,
			// i.e flattened = append(flattened, mySlice...)
			elementV := reflect.ValueOf(element)
			for j := 0; j < elementV.Len(); j++ {
				flattened = append(
					flattened, elementV.Index(j).Interface())
			}
		} else {
			flattened = append(flattened, element)
		}
	}
	return flattened, nil
}

func (intr *treeInterpreter) sliceWithReflection(node ASTNode, value interface{}) (interface{}, error) {
	v := reflect.ValueOf(value)
	parts := node.value.([]*int)
	sliceParams := make([]sliceParam, 3)
	for i, part := range parts {
		if part != nil {
			sliceParams[i].Specified = true
			sliceParams[i].N = *part
		}
	}
	final := []interface{}{}
	for i := 0; i < v.Len(); i++ {
		element := v.Index(i).Interface()
		final = append(final, element)
	}
	return slice(final, sliceParams)
}

func (intr *treeInterpreter) filterProjectionWithReflection(node ASTNode, value interface{}) (interface{}, error) {
	compareNode := node.children[2]
	collected := []interface{}{}
	v := reflect.ValueOf(value)
	for i := 0; i < v.Len(); i++ {
		element := v.Index(i).Interface()
		result, err := intr.Execute(compareNode, element)
		if err != nil {
			return nil, err
		}
		if !isFalse(result) {
			current, err := intr.Execute(node.children[1], element)
			if err != nil {
				return nil, err
			}
			if current != nil {
				collected = append(collected, current)
			}
		}
	}
	return collected, nil
}

func (intr *treeInterpreter) projectWithReflection(node ASTNode, value interface{}) (interface{}, error) {
	collected := []interface{}{}
	v := reflect.ValueOf(value)
	for i := 0; i < v.Len(); i++ {
		element := v.Index(i).Interface()
		result, err := intr.Execute(node.children[1], element)
		if err != nil {
			return nil, err
		}
		if result != nil {
			collected = append(collected, result)
		}
	}
	return collected, nil
}
