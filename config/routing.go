package config

import (
	"errors"
	"reflect"
)

type Router string

const (
	RouterHTTPHeader Router = "http-header"
	RouterSNI        Router = "sni"
	RouterTCPRaw     Router = "tcp-raw"
	RouterUDPRaw     Router = "udp-raw"
)

func (r *Router) Decode(from reflect.Type, val interface{}) (any, error) {
	if from.Kind() != reflect.String {
		return val, nil // not applicable
	}

	if val == nil {
		return val, nil // nothing to decode
	}

	strVal, ok := val.(string)
	if !ok {
		return nil, errors.New("expected string value for Router")
	}

	if err := r.Set(strVal); err != nil {
		return nil, err
	}

	return r, nil
}

func (r *Router) IsValid() bool {
	switch *r {
	case RouterHTTPHeader, RouterSNI, RouterTCPRaw, RouterUDPRaw:
		return true
	default:
		return false
	}
}

func (r *Router) String() string {
	if r == nil {
		return ""
	}
	return string(*r)
}

func (r *Router) Set(value string) error {
	if value == "" {
		return nil
	}
	router := Router(value)
	if !router.IsValid() {
		return errors.New("invalid router type: " + value)
	}
	*r = router
	return nil
}
