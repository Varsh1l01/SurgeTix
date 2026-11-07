package idempotency

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"sort"
	"strconv"
	"strings"
)

// ErrInvalidJSON is returned for payloads that cannot be canonicalised:
// malformed JSON, trailing data, or a duplicate object key (whose meaning
// is ambiguous, so it can never hash "equal" to anything safely).
var ErrInvalidJSON = errors.New("idempotency: request is not canonicalisable JSON")

// CanonicalJSON rewrites a JSON document into one canonical form, so that
// semantically equivalent requests produce identical bytes (AMB-48):
//   - object keys sorted (byte order) at every level;
//   - no insignificant whitespace;
//   - strings re-encoded uniformly (escapes such as A become "A");
//     no HTML escaping;
//   - numbers normalised to one spelling of their exact value: 1, 1.0,
//     1e0 and 10e-1 are all "1", and 0.5 and 5e-1 are both "0.5".
//
// Array order is significant and preserved. Callers whose arrays are
// semantically sets (e.g. seat_ids) must sort them before hashing.
func CanonicalJSON(doc []byte) ([]byte, error) {
	dec := json.NewDecoder(bytes.NewReader(doc))
	dec.UseNumber()
	v, err := readValue(dec)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidJSON, err)
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, fmt.Errorf("%w: trailing data after the JSON value", ErrInvalidJSON)
	}
	var buf bytes.Buffer
	if err := writeValue(&buf, v); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidJSON, err)
	}
	return buf.Bytes(), nil
}

// RequestHash is SHA-256 over the scope and the canonical JSON request. The
// scope is included so equal bodies sent to different operations never
// share a hash.
func RequestHash(scope string, request []byte) ([]byte, error) {
	if scope == "" {
		return nil, errors.New("idempotency: scope is required")
	}
	c, err := CanonicalJSON(request)
	if err != nil {
		return nil, err
	}
	h := sha256.New()
	h.Write([]byte(scope))
	h.Write([]byte{0}) // separator: scope bytes can never run into body bytes
	h.Write(c)
	return h.Sum(nil), nil
}

type object struct {
	keys []string
	vals map[string]any
}

// readValue decodes one JSON value, detecting duplicate object keys.
func readValue(dec *json.Decoder) (any, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	switch t := tok.(type) {
	case json.Delim:
		switch t {
		case '{':
			o := object{vals: map[string]any{}}
			for dec.More() {
				kt, err := dec.Token()
				if err != nil {
					return nil, err
				}
				k := kt.(string)
				if _, dup := o.vals[k]; dup {
					return nil, fmt.Errorf("duplicate object key %q", k)
				}
				v, err := readValue(dec)
				if err != nil {
					return nil, err
				}
				o.keys = append(o.keys, k)
				o.vals[k] = v
			}
			if _, err := dec.Token(); err != nil { // '}'
				return nil, err
			}
			return o, nil
		case '[':
			var arr []any
			for dec.More() {
				v, err := readValue(dec)
				if err != nil {
					return nil, err
				}
				arr = append(arr, v)
			}
			if _, err := dec.Token(); err != nil { // ']'
				return nil, err
			}
			return arr, nil
		}
		return nil, fmt.Errorf("unexpected delimiter %v", t)
	default:
		return tok, nil // string, json.Number, bool, nil
	}
}

func writeValue(buf *bytes.Buffer, v any) error {
	switch x := v.(type) {
	case object:
		keys := append([]string(nil), x.keys...)
		sort.Strings(keys)
		buf.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				buf.WriteByte(',')
			}
			writeString(buf, k)
			buf.WriteByte(':')
			if err := writeValue(buf, x.vals[k]); err != nil {
				return err
			}
		}
		buf.WriteByte('}')
	case []any:
		buf.WriteByte('[')
		for i, e := range x {
			if i > 0 {
				buf.WriteByte(',')
			}
			if err := writeValue(buf, e); err != nil {
				return err
			}
		}
		buf.WriteByte(']')
	case string:
		writeString(buf, x)
	case json.Number:
		n, err := canonicalNumber(string(x))
		if err != nil {
			return err
		}
		buf.WriteString(n)
	case bool:
		buf.WriteString(strconv.FormatBool(x))
	case nil:
		buf.WriteString("null")
	default:
		return fmt.Errorf("unexpected JSON value %T", v)
	}
	return nil
}

func writeString(buf *bytes.Buffer, s string) {
	enc := json.NewEncoder(buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(s)           // cannot fail for a string
	buf.Truncate(buf.Len() - 1) // Encode appends '\n'
}

// canonicalNumber renders a JSON number's EXACT value in one spelling: an
// integer when the value is integral, otherwise the shortest exact decimal.
// It uses arbitrary precision, so no float rounding can make two different
// values equal.
func canonicalNumber(lit string) (string, error) {
	r, ok := new(big.Rat).SetString(lit)
	if !ok {
		return "", fmt.Errorf("invalid number %q", lit)
	}
	if r.IsInt() {
		return r.Num().String(), nil
	}
	// A JSON number is a finite decimal, so its value has a finite decimal
	// expansion. Find the smallest precision that reproduces it exactly.
	for prec := 1; prec <= 4096; prec++ {
		s := r.FloatString(prec)
		if back, _ := new(big.Rat).SetString(s); back.Cmp(r) == 0 {
			return strings.TrimRight(strings.TrimRight(s, "0"), "."), nil
		}
	}
	return "", fmt.Errorf("number %q has too many decimal places", lit)
}
