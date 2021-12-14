package ssmconfig

import (
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ssm"
	"github.com/aws/aws-sdk-go/service/ssm/ssmiface"
)

// queueObject is a private struct used to walk through the config struct.
type queueObject struct {
	spec      reflect.Value
	specType  reflect.Type
	field     reflect.Value
	fieldType reflect.StructField
	prefix    string
}

// Process processes the config struct and any fields with ssmparam tag will be filled.
// errors if any fields cannot be
func Process(ssmsvc ssmiface.SSMAPI, prefix string, spec interface{}) error {
	s := reflect.ValueOf(spec)

	// requires ptr type
	if s.Kind() != reflect.Ptr {
		return errors.New("spec must be non-nil pointer")
	}
	s = s.Elem()

	// requires ptr to point to a struct
	if s.Kind() != reflect.Struct {
		return errors.New("spec must be a struct type")
	}

	typeOfSpec := s.Type()
	queue := []queueObject{}
	for i := 0; i < s.NumField(); i++ {
		f := s.Field(i)
		ftype := typeOfSpec.Field(i)
		queue = append(queue, queueObject{
			spec:      s,
			specType:  typeOfSpec,
			field:     f,
			fieldType: ftype,
			prefix:    prefix,
		})
	}

	infos := map[string]reflect.Value{}
	for {
		// quit if queue is empty
		if len(queue) == 0 {
			break
		}

		// pop
		q := queue[0]     // Pop
		queue = queue[1:] // Dequeue

		// if the field can't be changed skip
		if !q.field.CanSet() {
			continue
		}

		ssmparam := q.fieldType.Tag.Get("ssmparam")

		// if field is a sub struct, then we add those fields to the queue
		if q.field.Kind() == reflect.Struct {
			embeddedPtr := q.field.Addr().Interface()
			s := reflect.ValueOf(embeddedPtr).Elem()
			typeOfSpec := s.Type()
			for i := 0; i < s.NumField(); i++ {
				f := s.Field(i)
				ftype := typeOfSpec.Field(i)
				queue = append(queue, queueObject{
					spec:      s,
					specType:  typeOfSpec,
					field:     f,
					fieldType: ftype,
					// add prefix if the struct has an ssmparam tag so we can build out the ssm in parts
					prefix: q.prefix + ssmparam,
				})
			}
			continue
		}
		// if no ssm param then skip
		if ssmparam == "" {
			continue
		}
		key := q.prefix + ssmparam
		infos[key] = q.field
	}

	// makes a list of names from infos keys
	names := make([]*string, len(infos))
	i := 0
	for k := range infos {
		names[i] = aws.String(k)
		i++
	}

	// gets ssm parameters
	out, err := ssmsvc.GetParameters(&ssm.GetParametersInput{
		Names:          names,
		WithDecryption: aws.Bool(true),
	})
	if err != nil {
		return err
	}

	// fills in values of the config stuct with values gotten from ssm parameters.
	for _, param := range out.Parameters {
		f, ok := infos[aws.StringValue(param.Name)]
		if !ok { // got something back we didn't ask for
			continue
		}
		if err := processField(aws.StringValue(param.Value), f); err != nil {
			return err
		}
	}
	return nil
}

func processField(value string, field reflect.Value) error {
	typ := field.Type()

	switch typ.Kind() {
	case reflect.String:
		field.SetString(value)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		var (
			val int64
			err error
		)
		if field.Kind() == reflect.Int64 && typ.PkgPath() == "time" && typ.Name() == "Duration" {
			var d time.Duration
			d, err = time.ParseDuration(value)
			val = int64(d)
		} else {
			val, err = strconv.ParseInt(value, 0, typ.Bits())
		}
		if err != nil {
			return err
		}

		field.SetInt(val)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		val, err := strconv.ParseUint(value, 0, typ.Bits())
		if err != nil {
			return err
		}
		field.SetUint(val)
	case reflect.Bool:
		val, err := strconv.ParseBool(value)
		if err != nil {
			return err
		}
		field.SetBool(val)
	case reflect.Float32, reflect.Float64:
		val, err := strconv.ParseFloat(value, typ.Bits())
		if err != nil {
			return err
		}
		field.SetFloat(val)
	case reflect.Slice:
		sl := reflect.MakeSlice(typ, 0, 0)
		if typ.Elem().Kind() == reflect.Uint8 {
			sl = reflect.ValueOf([]byte(value))
		} else if len(strings.TrimSpace(value)) != 0 {
			vals := strings.Split(value, ",")
			sl = reflect.MakeSlice(typ, len(vals), len(vals))
			for i, val := range vals {
				err := processField(val, sl.Index(i))
				if err != nil {
					return err
				}
			}
		}
		field.Set(sl)
	case reflect.Map:
		mp := reflect.MakeMap(typ)
		if len(strings.TrimSpace(value)) != 0 {
			pairs := strings.Split(value, ",")
			for _, pair := range pairs {
				kvpair := strings.Split(pair, ":")
				if len(kvpair) != 2 {
					return fmt.Errorf("invalid map item: %q", pair)
				}
				k := reflect.New(typ.Key()).Elem()
				err := processField(kvpair[0], k)
				if err != nil {
					return err
				}
				v := reflect.New(typ.Elem()).Elem()
				err = processField(kvpair[1], v)
				if err != nil {
					return err
				}
				mp.SetMapIndex(k, v)
			}
		}
		field.Set(mp)
	}

	return nil
}
