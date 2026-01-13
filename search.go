package cbor

import (
	"fmt"
	"io"
	"reflect"
	"strconv"
)

// Searcher offers Search and SearchValue functions that allow respectively to
// find and decode a value at a given JSON pointer like path.
//
// With the following example:
// { "foo": { "bar": ["a", "b"], "baz": "ok", "ints": {1: "one", 2: "two"} }}
// SearchValue for ["foo","bar","0"] returns "a", ["foo","baz"] returns "ok" and
// ["foo", "ints", "0"] returns "one".
//
// Search retrieve the offset in the cbor to allow decoding the pointed value. It
// offers optional functionalities to obtain the array length or map array retrieved
// at the given path and - for maps - to retrieve the first or all keys.
// Search and SearchValue have both an optional callback function to convert a
// struct key map into a string - to match search keys.
type Searcher struct {
	dec *ReadSeekDecoder
}

type ReadSeekDecoder struct {
	opts DecOptions
	dec  *Decoder
}

func (rsd *ReadSeekDecoder) DecOptions() DecOptions {
	return rsd.opts
}

func (rsd *ReadSeekDecoder) ReadNext() (int, error) {
	return rsd.dec.readNext()
}

func (rsd *ReadSeekDecoder) SkipReadNext() error {
	err := rsd.dec.Skip()
	if err != nil {
		return err
	}
	_, err = rsd.dec.readNext()
	return err
}

func (rsd *ReadSeekDecoder) Reset() {
	rsd.dec.d.reset(rsd.dec.buf[rsd.dec.off:])
}

func (rsd *ReadSeekDecoder) Offset() int {
	return rsd.dec.d.off
}

func (rsd *ReadSeekDecoder) Seek(off int) {
	rsd.dec.d.off = off
}

func (rsd *ReadSeekDecoder) NextCBORType() cborType {
	return rsd.dec.d.nextCBORType()
}

func (rsd *ReadSeekDecoder) GetHead() (t cborType, ai byte, val uint64) {
	return rsd.dec.d.getHead()
}

func (rsd *ReadSeekDecoder) GetHeadWithIndefiniteLengthFlag() (
	t cborType,
	ai byte,
	val uint64,
	indefiniteLength bool) {
	return rsd.dec.d.getHeadWithIndefiniteLengthFlag()
}

func (rsd *ReadSeekDecoder) NumOfItemsUntilBreak() int {
	return rsd.dec.d.numOfItemsUntilBreak()
}

func (rsd *ReadSeekDecoder) FoundBreak() bool {
	return rsd.dec.d.foundBreak()
}

func (rsd *ReadSeekDecoder) Skip() {
	rsd.dec.d.skip()
}

func (rsd *ReadSeekDecoder) Value(v any) error {
	return rsd.dec.d.value(v)
}

func (rsd *ReadSeekDecoder) Parse(skipSelfDescribedTag bool) (any, error) {
	return rsd.dec.d.parse(skipSelfDescribedTag)
}

// SearchInfo is returned by a call to Search
type SearchInfo struct {
	Offset   int      // offset in cbor bytes at which the cbor element is found
	Size     int      // count of cbor data bytes for the found element
	CborType string   // cbor type (as a string) of the retrieved element
	Length   int      // when Length option is true: count of items if the element is an array or a map
	MapKeys  []string // when MapKeyFirst or MapKeyAll option is true: map key(s) if the element is a map
}

// KeyStructStringerFn is a user provided function that is invoked for structs used as map keys.
// Parameter:
// - keys: the parent path at which the key is found
// - k: the key to convert
// The function must return the key as a string and true if it handled the value.
//
// The key ('k') may be of type:
// - map[interface{}]interface{} or map[string]interface{} depending on the option used for default map or
// - the actual struct type if the encoder/decoder was configured with a tag for it
// The returned key can be used inside a 'keys' parameter used in one of the Search functions.
type KeyStructStringerFn func(keys []string, k any) (string, bool)

// SearchOption defines optional values for a search.
type SearchOption struct {
	Size              bool                // true to retrieve the count of cbor bytes of the retrieved element
	Length            bool                // true to retrieve the count of elements when the element is an array of a map
	MapKeyFirst       bool                // true to retrieve the first map key (if the retrieved element is a map)
	MapKeyAll         bool                // true to retrieve all map keys (if the retrieved element is a map)
	KeyStructStringer KeyStructStringerFn // function to convert a struct key map to a string
}

func (o SearchOption) arrayMapOption(t cborType) bool {
	switch t {
	case cborTypeArray:
		return o.Length
	case cborTypeMap:
		return o.Length || o.MapKeyFirst || o.MapKeyAll
	}
	return false
}

var (
	noOption = SearchOption{}
	noInfo   = SearchInfo{}
)

// NewSearcher returns a new searcher that reads from r using dm DecMode.
func (dm *decMode) NewSearcher(r io.Reader) (*Searcher, error) {
	dec := &ReadSeekDecoder{
		opts: dm.DecOptions(),
		dec:  &Decoder{r: r, d: decoder{dm: dm}},
	}
	siz, err := dec.ReadNext()
	if err != nil {
		return nil, err
	}
	_ = siz
	return &Searcher{
		dec: dec,
	}, nil
}

func (sea *Searcher) skipSelfDescribedTag() {
	for sea.dec.NextCBORType() == cborTypeTag {
		off := sea.dec.Offset()
		_, _, tagNum := sea.dec.GetHead()
		if tagNum != tagNumSelfDescribedCBOR {
			sea.dec.Seek(off)
			break
		}
	}
}

func (sea *Searcher) arrayMapInfo(t cborType, keys []string, opt SearchOption, info SearchInfo) (SearchInfo, error) {
	savedOff := sea.dec.Offset()

	switch t {
	case cborTypeArray:
		_, _, val, indefiniteLength := sea.dec.GetHeadWithIndefiniteLengthFlag()
		hasSize := !indefiniteLength
		count := int(val)
		if !hasSize {
			count = sea.dec.NumOfItemsUntilBreak()
		}
		info.Length = count
	case cborTypeMap:
		_, _, val, indefiniteLength := sea.dec.GetHeadWithIndefiniteLengthFlag()
		hasSize := !indefiniteLength
		count := int(val)

		length := 0
		var mapKeys []string
	out:
		for i := 0; (hasSize && i < count) || (!hasSize && !sea.dec.FoundBreak()); i++ {
			length++
			switch {
			case (i == 0 && opt.MapKeyFirst) || opt.MapKeyAll:
				key, err := sea.parseCborMapKey(keys, opt)
				if err != nil {
					return info, err
				}
				mapKeys = append(mapKeys, key)
				if !opt.Length && !opt.MapKeyAll {
					break out // exit if we just want the first key
				}
			default:
				// skip CBOR map key.
				sea.dec.Skip()
			}
			// skip CBOR map value.
			sea.dec.Skip()
		}
		if opt.Length {
			info.Length = length
		}
		info.MapKeys = mapKeys
	}

	sea.dec.Seek(savedOff)
	return info, nil
}

// Skip invokes Skip on the underlying decoder.
func (sea *Searcher) Skip() error {
	return sea.dec.SkipReadNext()
}

// SearchValue searches for the element pointed to by the given path (JSON
// pointer like) and decode it into the given value.
func (sea *Searcher) SearchValue(keys []string, v any, opts ...SearchOption) error {
	_, err := sea.Search(keys, opts...)
	if err != nil {
		return err
	}
	return sea.dec.Value(v)
}

// parseCborMapKey convert CBOR map key to string
func (sea *Searcher) parseCborMapKey(keys []string, opt SearchOption) (string, error) {
	k, err := sea.dec.Parse(true)
	if err != nil {
		return "", newSearchErrorf("error parsing map key: (%v)", err)
	}

	rv := reflect.ValueOf(k)
	if !isHashableValue(rv) {
		var converted bool
		if sea.dec.DecOptions().MapKeyByteString == MapKeyByteStringAllowed {
			k, converted = convertByteSliceToByteString(k)
		}
		if !converted {
			if rv.Kind() == reflect.Struct || rv.Kind() == reflect.Map {
				if opt.KeyStructStringer != nil {
					k, converted = opt.KeyStructStringer(keys, k)
				}
			}
			if !converted {
				return "", newSearchErrorf("invalid map key: (%v)", rv.Type().String())
			}
		}
	}

	key := ""
	switch ks := k.(type) {
	case string:
		key = ks
	case ByteString:
		key = string(ks)
	default:
		converted := false
		switch rv.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
			reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
			reflect.Float32, reflect.Float64:

			key = fmt.Sprintf("%v", k)
			converted = true

		case reflect.Struct:
			if opt.KeyStructStringer != nil {
				key, converted = opt.KeyStructStringer(keys, k)
			}
		default:
		}
		if !converted {
			return "", newSearchErrorf("could not convert map key: %#v (type %v)",
				k,
				rv.Type().String())
		}
	}
	return key, nil
}

// Search returns a SearchInfo containing the offset of the element pointed to
// by the given path (JSON pointer like) and its cbor type as a string or an error.
func (sea *Searcher) Search(keys []string, opts ...SearchOption) (info SearchInfo, err error) {

	sea.dec.Reset()

	if keys == nil {
		keys = []string{}
	}

	opt := noOption
	if len(opts) > 0 {
		opt = opts[0]
	}

	// strip self-described CBOR tag number.
	sea.skipSelfDescribedTag()

	idx := 0

	for {
		offset := sea.dec.Offset()
		t := sea.dec.NextCBORType()

		if idx > len(keys)-1 {
			ret := SearchInfo{
				Offset:   offset,
				CborType: t.String(),
			}
			if opt.Size {
				sea.dec.Skip()
				endOffset := sea.dec.Offset()
				ret.Size = endOffset - offset
				sea.dec.Seek(offset)
			}
			if opt.arrayMapOption(t) {
				ret, err = sea.arrayMapInfo(t, keys, opt, ret)
				if err != nil {
					return noInfo, err
				}
			}
			return ret, nil
		}

		switch t {
		case cborTypeTag:
			_, _, tagNum := sea.dec.GetHead()
			contentOff := sea.dec.Offset()

			switch tagNum {
			case tagNumRFC3339Time, tagNumEpochTime, tagNumUnsignedBignum, tagNumNegativeBignum,
				tagNumExpectedLaterEncodingBase64URL, tagNumExpectedLaterEncodingBase64,
				tagNumExpectedLaterEncodingBase16:
				return noInfo, newSearchErrorf("not found, path: %s", keys[:idx+1])
			default:
				// parse tag content
				sea.dec.Seek(contentOff)
				continue
			}

		case cborTypeArray:
			i, err := strconv.ParseInt(keys[idx], 10, 32)
			if err != nil {
				return noInfo, newSearchErrorf("invalid array index, path: %s", keys[:idx+1])
			}
			aidx := int(i)

			_, _, val, indefiniteLength := sea.dec.GetHeadWithIndefiniteLengthFlag()
			hasSize := !indefiniteLength
			count := int(val)
			if !hasSize {
				count = sea.dec.NumOfItemsUntilBreak()
			}

			if aidx < 0 {
				// treat negative index as offset from the end
				// i.e -1 is last element
				aidx = count + aidx
			}
			if aidx >= count || aidx < 0 {
				return noInfo, newSearchErrorf("array index out of range, path: %s", keys[:idx+1])
			}

			for i := 0; i <= aidx; i++ {
				sea.skipSelfDescribedTag()
				if i < aidx {
					sea.dec.Skip()
				}
			}

		case cborTypeMap:
			_, _, val, indefiniteLength := sea.dec.GetHeadWithIndefiniteLengthFlag()
			hasSize := !indefiniteLength
			count := int(val)

			found := false
			for i := 0; (hasSize && i < count) || (!hasSize && !sea.dec.FoundBreak()); i++ {
				key, err := sea.parseCborMapKey(keys[:idx+1], opt)
				if err != nil {
					return noInfo, newSearchErrorf("Error at path: %s (%v)", keys[:idx+1], err.Error())
				}
				if key == keys[idx] {
					found = true
					break
				}
				// skip CBOR map value.
				sea.dec.Skip()
			}

			if !found {
				return noInfo, newSearchErrorf("map key not found, path: %s", keys[:idx+1])
			}

		}
		idx++
	}

}

func newSearchErrorf(format string, args ...interface{}) *SearchError {
	return &SearchError{msg: fmt.Sprintf(format, args...)}
}

// SearchError are errors returned by the Searcher
type SearchError struct {
	msg string
}

func (e *SearchError) Error() string {
	return e.msg
}
