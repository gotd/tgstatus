// Code generated by ogen, DO NOT EDIT.

package oas

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-faster/errors"
	"github.com/go-faster/jx"
	"github.com/google/uuid"
	"github.com/ogen-go/ogen/conv"
	ht "github.com/ogen-go/ogen/http"
	"github.com/ogen-go/ogen/json"
	"github.com/ogen-go/ogen/otelogen"
	"github.com/ogen-go/ogen/uri"
	"github.com/ogen-go/ogen/validate"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

// No-op definition for keeping imports.
var (
	_ = context.Background()
	_ = fmt.Stringer(nil)
	_ = strings.Builder{}
	_ = errors.Is
	_ = sort.Ints
	_ = http.MethodGet
	_ = io.Copy
	_ = json.Marshal
	_ = bytes.NewReader
	_ = strconv.ParseInt
	_ = time.Time{}
	_ = conv.ToInt32
	_ = uuid.UUID{}
	_ = uri.PathEncoder{}
	_ = url.URL{}
	_ = math.Mod
	_ = validate.Int{}
	_ = ht.NewRequest
	_ = net.IP{}
	_ = otelogen.Version
	_ = trace.TraceIDFromHex
	_ = otel.GetTracerProvider
	_ = metric.NewNoopMeterProvider
	_ = regexp.MustCompile
	_ = jx.Null
	_ = sync.Pool{}
)

// Encode implements json.Marshaler.
func (s Location) Encode(e *jx.Encoder) {
	e.ObjStart()

	e.FieldStart("id")
	e.Int(s.ID)

	e.FieldStart("services")
	e.ArrStart()
	for _, elem := range s.Services {
		elem.Encode(e)
	}
	e.ArrEnd()
	e.ObjEnd()
}

// Decode decodes Location from json.
func (s *Location) Decode(d *jx.Decoder) error {
	if s == nil {
		return errors.New(`invalid: unable to decode Location to nil`)
	}
	return d.ObjBytes(func(d *jx.Decoder, k []byte) error {
		switch string(k) {
		case "id":
			v, err := d.Int()
			s.ID = int(v)
			if err != nil {
				return err
			}
		case "services":
			s.Services = nil
			if err := d.Arr(func(d *jx.Decoder) error {
				var elem Service
				if err := elem.Decode(d); err != nil {
					return err
				}
				s.Services = append(s.Services, elem)
				return nil
			}); err != nil {
				return err
			}
		default:
			return d.Skip()
		}
		return nil
	})
}

// Encode encodes net.IP as json.
func (o OptIP) Encode(e *jx.Encoder) {
	json.EncodeIP(e, o.Value)
}

// Decode decodes net.IP from json.
func (o *OptIP) Decode(d *jx.Decoder) error {
	if o == nil {
		return errors.New(`invalid: unable to decode OptIP to nil`)
	}
	switch d.Next() {
	case jx.String:
		o.Set = true
		v, err := json.DecodeIP(d)
		if err != nil {
			return err
		}
		o.Value = v
		return nil
	default:
		return errors.Errorf(`unexpected type %q while reading OptIP`, d.Next())
	}
}

// Encode implements json.Marshaler.
func (s Service) Encode(e *jx.Encoder) {
	e.ObjStart()

	e.FieldStart("status")
	s.Status.Encode(e)
	if s.IPV4.Set {
		e.FieldStart("ip_v4")
		s.IPV4.Encode(e)
	}
	if s.IPV6.Set {
		e.FieldStart("ip_v6")
		s.IPV6.Encode(e)
	}
	e.ObjEnd()
}

// Decode decodes Service from json.
func (s *Service) Decode(d *jx.Decoder) error {
	if s == nil {
		return errors.New(`invalid: unable to decode Service to nil`)
	}
	return d.ObjBytes(func(d *jx.Decoder, k []byte) error {
		switch string(k) {
		case "status":
			if err := s.Status.Decode(d); err != nil {
				return err
			}
		case "ip_v4":
			s.IPV4.Reset()
			if err := s.IPV4.Decode(d); err != nil {
				return err
			}
		case "ip_v6":
			s.IPV6.Reset()
			if err := s.IPV6.Decode(d); err != nil {
				return err
			}
		default:
			return d.Skip()
		}
		return nil
	})
}

// Encode encodes ServiceStatus as json.
func (s ServiceStatus) Encode(e *jx.Encoder) {
	e.Str(string(s))
}

// Decode decodes ServiceStatus from json.
func (s *ServiceStatus) Decode(d *jx.Decoder) error {
	if s == nil {
		return errors.New(`invalid: unable to decode ServiceStatus to nil`)
	}
	v, err := d.Str()
	if err != nil {
		return err
	}
	*s = ServiceStatus(v)
	return nil
}

// Encode implements json.Marshaler.
func (s Status) Encode(e *jx.Encoder) {
	e.ObjStart()
	if s.Locations != nil {
		e.FieldStart("locations")
		e.ArrStart()
		for _, elem := range s.Locations {
			elem.Encode(e)
		}
		e.ArrEnd()
	}
	e.ObjEnd()
}

// Decode decodes Status from json.
func (s *Status) Decode(d *jx.Decoder) error {
	if s == nil {
		return errors.New(`invalid: unable to decode Status to nil`)
	}
	return d.ObjBytes(func(d *jx.Decoder, k []byte) error {
		switch string(k) {
		case "locations":
			s.Locations = nil
			if err := d.Arr(func(d *jx.Decoder) error {
				var elem Location
				if err := elem.Decode(d); err != nil {
					return err
				}
				s.Locations = append(s.Locations, elem)
				return nil
			}); err != nil {
				return err
			}
		default:
			return d.Skip()
		}
		return nil
	})
}
