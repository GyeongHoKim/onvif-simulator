//go:build rpicam && linux && (arm || arm64)

package rpicamera

import (
	"encoding/base64"
	"reflect"
	"strconv"
	"strings"
)

// serialize encodes upstreamParams into the space-separated `key:value` form
// the mtxrpicam helper parses on its control pipe (verbatim from mediamtx).
func (p upstreamParams) serialize() []byte {
	rv := reflect.ValueOf(p)
	rt := rv.Type()
	nf := rv.NumField()
	ret := make([]string, nf)

	for i := range nf {
		entry := rt.Field(i).Name + ":"
		f := rv.Field(i)
		v := f.Interface()

		switch v := v.(type) {
		case uint32:
			entry += strconv.FormatUint(uint64(v), 10)
		case float32:
			entry += strconv.FormatFloat(float64(v), 'f', -1, 32)
		case string:
			entry += base64.StdEncoding.EncodeToString([]byte(v))
		case bool:
			if f.Bool() {
				entry += "1"
			} else {
				entry += "0"
			}
		default:
			panic("unhandled type")
		}

		ret[i] = entry
	}

	return []byte(strings.Join(ret, " "))
}
