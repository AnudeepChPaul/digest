package review

import (
	"reflect"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

var lineEndings = strings.NewReplacer("\r\n", "\n", "\r", "\n")

func PlainText(text string) string {
	stripped := ansi.Strip(lineEndings.Replace(text))
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' {
			return r
		}
		if r < 0x20 || (r >= 0x7f && r <= 0x9f) {
			return -1
		}
		return r
	}, stripped)
}

func plainTextFields(target any) {
	plainTextValue(reflect.ValueOf(target))
}

func plainTextValue(value reflect.Value) {
	switch value.Kind() {
	case reflect.Pointer, reflect.Interface:
		if !value.IsNil() {
			plainTextValue(value.Elem())
		}
	case reflect.Struct:
		for index := range value.NumField() {
			if value.Type().Field(index).IsExported() {
				plainTextValue(value.Field(index))
			}
		}
	case reflect.Slice, reflect.Array:
		for index := range value.Len() {
			plainTextValue(value.Index(index))
		}
	case reflect.String:
		if value.CanSet() {
			value.SetString(PlainText(value.String()))
		}
	}
}
