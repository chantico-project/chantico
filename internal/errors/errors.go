package errors

import (
	"fmt"
)

const ( // Different values of kind
	KindConfigMap = "configmap"
	KindSecret    = "secret"
)

type GetError struct {
	Kind string // e.g. "configmap" or "secret"
	Name string
	Err  error
}

func (e *GetError) Error() string {
	return fmt.Sprintf("get %s %q: %v", e.Kind, e.Name, e.Err)
}

func (e *GetError) Unwrap() error {
	return e.Err
}

type MissingKeyError struct {
	Kind string // e.g. "configmap" or "secret"
	Name string
	Key  string
}

func (e *MissingKeyError) Error() string {
	return fmt.Sprintf("%s %q does not contain key %q", e.Kind, e.Name, e.Key)
}

type MissingOptionError struct {
	Parent  string
	Options []string
}

func (e *MissingOptionError) Error() string {
	return fmt.Sprintf("%s must specify one of: %v", e.Parent, e.Options)
}

type NotImplementedError struct{}

func (e *NotImplementedError) Error() string {
	return "not implemented"
}
