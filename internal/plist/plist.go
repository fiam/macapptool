package plist

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"reflect"

	"howett.net/plist"
)

const (
	CFBundleIdentifier         = "CFBundleIdentifier"
	CFBundleName               = "CFBundleName"
	CFBundleShortVersionString = "CFBundleShortVersionString"
)

type ErrKeyNotFound struct {
	Key string
}

func (e *ErrKeyNotFound) Error() string {
	return fmt.Sprintf("key %q not found", e.Key)
}

func typeName(t reflect.Type) string {
	pkgPath := t.PkgPath()
	if pkgPath == "main" || pkgPath == "" {
		return t.Name()
	}
	return fmt.Sprintf("%s.%s", pkgPath, t.Name())
}

type ErrInvalidType struct {
	Key      string
	Expected reflect.Type
	Type     reflect.Type
}

func (e *ErrInvalidType) Error() string {
	var prefix string
	if e.Key != "" {
		prefix = fmt.Sprintf("key %q has invalid type: ", e.Key)
	}
	return fmt.Sprintf("%sexpecting value of type %s, got %s instead", prefix, typeName(e.Expected), typeName(e.Type))
}

type PList struct {
	data map[string]interface{}
}

func New(r io.Reader) (*PList, error) {
	var rs io.ReadSeeker
	if rrs, ok := r.(io.ReadSeeker); ok {
		rs = rrs
	} else {
		data, err := ioutil.ReadAll(r)
		if err != nil {
			return nil, err
		}
		rs = bytes.NewReader(data)
	}
	var values map[string]interface{}
	dec := plist.NewDecoder(rs)
	if err := dec.Decode(&values); err != nil {
		return nil, err
	}
	return &PList{data: values}, nil
}

func NewFile(path string) (*PList, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return New(f)
}

func (pl *PList) stringKey(key string) (string, error) {
	value, found := pl.data[key]
	if !found {
		return "", &ErrKeyNotFound{Key: key}
	}
	s, ok := value.(string)
	if !ok {
		return "", &ErrInvalidType{Key: key, Expected: reflect.TypeOf(""), Type: reflect.TypeOf(value)}
	}
	return s, nil
}

func (pl *PList) BundleName() (string, error) {
	return pl.stringKey(CFBundleName)
}

func (pl *PList) BundleIdentifier() (string, error) {
	return pl.stringKey(CFBundleIdentifier)
}

func (pl *PList) BundleShortVersionString() (string, error) {
	return pl.stringKey(CFBundleShortVersionString)
}
