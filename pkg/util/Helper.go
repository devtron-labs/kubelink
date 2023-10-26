package util

import (
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"hash"
	"hash/fnv"
	coreV1 "k8s.io/api/core/v1"
	"reflect"
)

const alphanums = "bcdfghjklmnpqrstvwxz2456789"

func IsMapSubset(mapSet interface{}, mapSubset interface{}) bool {

	mapSetValue := reflect.ValueOf(mapSet)
	mapSubsetValue := reflect.ValueOf(mapSubset)

	if mapSetValue.Kind() != reflect.Map || mapSubsetValue.Kind() != reflect.Map {
		return false
	}
	if reflect.TypeOf(mapSetValue) != reflect.TypeOf(mapSubsetValue) {
		return false
	}
	if len(mapSubsetValue.MapKeys()) == 0 {
		return true
	}

	iterMapSubset := mapSubsetValue.MapRange()

	for iterMapSubset.Next() {
		k := iterMapSubset.Key()
		v := iterMapSubset.Value()

		if v2 := mapSetValue.MapIndex(k); !v2.IsValid() || v.Interface() != v2.Interface() {
			return false
		}
	}

	return true
}

func ComputeHash(template *coreV1.PodTemplateSpec) string {
	podTemplateSpecHasher := fnv.New32a()
	DeepHashObject(podTemplateSpecHasher, *template)
	return SafeEncodeString(fmt.Sprint(podTemplateSpecHasher.Sum32()))
}

func SafeEncodeString(s string) string {
	r := make([]byte, len(s))
	for i, b := range []rune(s) {
		r[i] = alphanums[(int(b) % len(alphanums))]
	}
	return string(r)
}

func DeepHashObject(hasher hash.Hash, objectToWrite interface{}) {
	hasher.Reset()
	printer := spew.ConfigState{
		Indent:         " ",
		SortKeys:       true,
		DisableMethods: true,
		SpewKeys:       true,
	}
	_, err := printer.Fprintf(hasher, "%#v", objectToWrite)
	if err != nil {
		fmt.Println(err)
	}
}
