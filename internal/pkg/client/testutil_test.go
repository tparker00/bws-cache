package client

import "errors"

func errorsNew(s string) error { return errors.New(s) }
func strPtr(s string) *string  { return &s }
