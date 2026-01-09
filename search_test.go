package cbor

import (
	"bytes"
	"log"
	"reflect"
	"testing"
)

type searchStruct struct {
	Name    string `json:"name"`
	Age     int    `json:"age"`
	Lengths []int  `json:"lengths"`
}
type searchTaggedStruct struct {
	Name    string `json:"name"`
	Age     int    `json:"age"`
	Lengths []int  `json:"lengths"`
}
type searchStructKey struct {
	Name string `json:"name"`
	Age  int    `json:"age"`
}
type searchTaggedStructKey struct {
	Name string `json:"name"`
	Age  int    `json:"age"`
}

func searchMap() map[string]any {
	return map[string]any{
		"cats": []string{"puppy", "miaow"},
		"animals": map[string]any{
			"cat": "puppy",
			"dog": "bobby",
		},
		"dummy": map[string]any{
			"animals": map[string]any{
				"cat":  "puppy",
				"dog":  "bobby",
				"cats": []string{"puppy", "miaow"},
			},
			"struct": &searchStruct{
				Name:    "bob",
				Age:     18,
				Lengths: []int{1, 2, -3},
			},
			"tagged-struct": &searchTaggedStruct{
				Name:    "bob",
				Age:     18,
				Lengths: []int{1, 2, -3},
			},
			"": "empty",
			"ints": map[int][]string{
				1: {"one"},
				2: {"two"},
				3: {"three"},
			},
		},
		"mappy": map[searchTaggedStructKey]string{
			searchTaggedStructKey{Name: "zero"}:         "zero",
			searchTaggedStructKey{Name: "one", Age: 12}: "one"},
	}
}

type searchTestContext struct {
	t   *testing.T
	enc EncMode
	dec DecMode
}

func (c *searchTestContext) mapBytes(m any) (any, []byte) {
	bb, err := c.enc.Marshal(m)
	if err != nil {
		c.t.Errorf("unexpected %v", err)
	}
	return m, bb
}

func newSearchTestContext(t *testing.T, defMapType ...reflect.Type) *searchTestContext {
	options := TagOptions{
		DecTag: DecTagOptional,
		EncTag: EncTagRequired,
	}
	tagSet := NewTagSet()
	err := tagSet.Add(options, reflect.TypeOf((*searchTaggedStruct)(nil)), 284) // dummy tag for test
	if err != nil {
		log.Fatal("invalid cbor tag", err, "tag", 284)
	}
	err = tagSet.Add(options, reflect.TypeOf((*searchTaggedStructKey)(nil)), 285) // dummy tag for test
	if err != nil {
		log.Fatal("invalid cbor tag", err, "tag", 285)
	}

	encOptions := CoreDetEncOptions()
	enc, err := encOptions.EncModeWithTags(tagSet)
	if err != nil {
		t.Errorf("unexpected %v", err)
	}

	var defaultMapType reflect.Type
	if len(defMapType) > 0 {
		defaultMapType = defMapType[0]
	}

	dec, err := DecOptions{
		DefaultMapType: defaultMapType,
	}.DecModeWithTags(tagSet)
	if err != nil {
		t.Errorf("unexpected %v", err)
	}

	return &searchTestContext{
		t:   t,
		enc: enc,
		dec: dec,
	}
}

func TestSearch(t *testing.T) {

	ctx := newSearchTestContext(t)
	_, mbytes := ctx.mapBytes(searchMap())
	//diag, err := Diagnose(mbytes)
	//if err != nil {
	//	t.Errorf("unexpected error %v", err)
	//}
	//fmt.Println(diag)

	sea, err := ctx.dec.NewSearcher(bytes.NewReader(mbytes))
	if err != nil {
		t.Errorf("unexpected %v", err)
	}

	info, err := sea.Search(nil)
	if err != nil {
		t.Errorf("unexpected %v", err)
	}
	if info.Offset != 0 {
		t.Fatalf("expected offset 0, got: %d", info.Offset)
	}
	if info.CborType != cborTypeMap.String() {
		t.Fatalf("expected offset 'map', got: %s", info.CborType)
	}

	type testCase struct {
		path    []string
		want    any
		wantErr bool
	}
	for i, tc := range []*testCase{
		{path: []string{"cats", "1"}, want: "miaow"},
		{path: []string{"cats", "2"}, wantErr: true},
		{path: []string{"animals", "cat"}, want: "puppy"},
		{path: []string{"animals", "dog"}, want: "bobby"},
		{path: []string{"animals", "lion"}, wantErr: true},
		{path: []string{"dummy", "animals", "cats", "1"}, want: "miaow"},
		{path: []string{"dummy", "tagged-struct", "name"}, want: "bob"},
		{path: []string{"dummy", "struct", "name"}, want: "bob"},
		{path: []string{"dummy", "struct", "age"}, want: uint64(18)},
		{path: []string{"dummy", "struct", "lengths", "0"}, want: uint64(1)},
		{path: []string{"dummy", "struct", "lengths", "1"}, want: uint64(2)},
		{path: []string{"dummy", "struct", "lengths", "2"}, want: int64(-3)},
		{path: []string{"dummy", "struct", "lengths", "-1"}, want: int64(-3)},
		{path: []string{"dummy", "struct", "lengths", "three"}, wantErr: true},
		{path: []string{"dummy", ""}, want: "empty"},
		{path: []string{"dummy", "ints", "1", "0"}, want: "one"},
		{path: []string{"dummy", "ints", "2", "0"}, want: "two"},
		{path: []string{"dummy", "ints", "3", "0"}, want: "three"},
		{path: []string{"mappy", "zero"}, wantErr: true},
	} {

		// test Search
		info, err := sea.Search(tc.path)
		if tc.wantErr {
			if err == nil {
				t.Errorf("case %d, %v", i, "expected error, got nil")
			}
		} else if err != nil {
			t.Errorf("case %d, %v", i, err)
		} else {
			var actual any
			err = sea.dec.d.value(&actual)
			if err != nil {
				t.Errorf("case %d, %v", i, err)
			}
			if actual != tc.want {
				t.Errorf("case %d, expected: %v, got: %v", i, tc.want, actual)
			}
		}

		// test SearchValue
		var actual any
		err = sea.SearchValue(tc.path, &actual)
		if tc.wantErr {
			if err == nil {
				t.Errorf("case %d, %v", i, "expected error, got nil")
			}
			continue
		}
		if err != nil {
			t.Errorf("case %d, %v", i, err)
		}
		if actual != tc.want {
			t.Errorf("case %d, expected: %v, got: %v", i, tc.want, actual)
		}

		// test with a regular decoder
		dec := ctx.dec.NewDecoder(bytes.NewReader(mbytes[info.Offset:]))
		var exp any
		err = dec.Decode(&exp)
		if err != nil {
			t.Errorf("case %d, %v", i, err)
		}
		if exp != tc.want {
			t.Errorf("case %d, expected: %v, got: %v", i, tc.want, exp)
		}
	}

}

func TestSearchTaggedStructPrefixedWithSelfDescribed(t *testing.T) {
	ctx := newSearchTestContext(t)

	// prefix with a tag 'SelfDescribedCBOR'
	buf := bytes.NewBuffer(make([]byte, 0))
	tg := Tag{Number: tagNumSelfDescribedCBOR}
	encodeHead(buf, byte(cborTypeTag), tg.Number)

	mbytes, err := ctx.enc.Marshal(&searchTaggedStruct{
		Name:    "bob",
		Age:     18,
		Lengths: []int{1, 2, -3},
	})
	if err != nil {
		t.Fatalf("unexpected %v", err)
	}

	_, err = buf.Write(mbytes)
	if err != nil {
		t.Fatalf("unexpected %v", err)
	}

	sea, err := ctx.dec.NewSearcher(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("unexpected %v", err)
	}

	var name interface{}
	err = sea.SearchValue([]string{"name"}, &name)
	if err != nil {
		t.Fatalf("unexpected %v", err)
	}

	if name != "bob" {
		t.Errorf("expected: bob, got: %v", name)
	}
}

// TestSearchLengthOptions tests the Length SearchOption
func TestSearchLengthOptions(t *testing.T) {
	t.Run("search-length", func(t *testing.T) {
		doTestSearchLengthOptions(t, false)
	})
	t.Run("search-length-prefixed-self-described", func(t *testing.T) {
		doTestSearchLengthOptions(t, true)
	})
}
func doTestSearchLengthOptions(t *testing.T, prefixWithSelfDescribed bool) {

	ctx := newSearchTestContext(t)
	_, mbytes := ctx.mapBytes(searchMap())

	// prefix with a tag 'SelfDescribedCBOR'
	buf := bytes.NewBuffer(make([]byte, 0))
	if prefixWithSelfDescribed {
		tg := Tag{Number: tagNumSelfDescribedCBOR}
		encodeHead(buf, byte(cborTypeTag), tg.Number)
	}

	_, err := buf.Write(mbytes)
	if err != nil {
		t.Fatalf("unexpected %v", err)
	}

	sea, err := ctx.dec.NewSearcher(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("unexpected %v", err)
	}

	type testCase struct {
		path []string
		want int
	}
	for i, tc := range []*testCase{
		{path: []string{"cats"}, want: 2},
		{path: []string{"animals"}, want: 2},
		{path: []string{"dummy", "animals"}, want: 3},
		{path: []string{"dummy", "animals", "cats"}, want: 2},
		{path: []string{"dummy", "tagged-struct", "lengths"}, want: 3},
		{path: []string{"dummy", "struct", "lengths"}, want: 3},
		{path: []string{"dummy", "ints"}, want: 3},
		{path: []string{"dummy", "ints", "2"}, want: 1},
	} {

		info, err := sea.Search(tc.path, SearchOption{Length: true})
		if err != nil {
			t.Errorf("case %d, %v", i, err)
		}
		if info.Length != tc.want {
			t.Errorf("case %d, expected: %v, got: %v", i, tc.want, info.Length)
		}
	}
}

// TestSearchMapKeyOptions tests the map-key SearchOption with int & float map key values
func TestSearchMapKeyOptions(t *testing.T) {

	ctx := newSearchTestContext(t)

	type testCase struct {
		mappy interface{}
		opt   SearchOption
		want  []string
	}

	for i, tc := range []*testCase{
		{
			mappy: map[string]any{"mappy": map[int]string{0: "zero", 1: "one"}},
			opt:   SearchOption{MapKeyFirst: true},
			want:  []string{"0"}},
		{
			mappy: map[string]any{"mappy": map[uint64]string{0: "zero", 1: "one"}},
			opt:   SearchOption{MapKeyAll: true},
			want:  []string{"0", "1"}},
		{
			mappy: map[string]any{"mappy": map[float64]string{0.01: "zero,zero,one", 0: "zero"}},
			opt:   SearchOption{MapKeyAll: true},
			want:  []string{"0", "0.01"}}, // order !
	} {
		_, mbytes := ctx.mapBytes(tc.mappy)
		sea, err := ctx.dec.NewSearcher(bytes.NewReader(mbytes))
		if err != nil {
			t.Errorf("unexpected %v", err)
		}

		info, err := sea.Search([]string{"mappy"}, tc.opt)
		if err != nil {
			t.Errorf("case %d, %v", i, err)
		}

		if len(info.MapKeys) != len(tc.want) {
			t.Errorf("case %d, wrong length, expected: %v, got: %v", i, tc.want, info.MapKeys)
		}
		for j := 0; j < len(tc.want); j++ {
			if info.MapKeys[j] != tc.want[j] {
				t.Errorf("case %d, unexpected key at %d, expected: %v, got: %v", i, j, tc.want, info.MapKeys)
			}
		}

		var val interface{}
		err = sea.SearchValue([]string{"mappy", info.MapKeys[0]}, &val)
		if err != nil {
			t.Errorf("case %d, %v", i, err)
		}
		if val != "zero" {
			t.Errorf("case %d, expected 'zero', got %v", i, val)
		}
	}
}

// TestSearchStructMapKeyOptions tests the map-key SearchOption with struct as map keys
func TestSearchStructMapKeyOptions(t *testing.T) {
	t.Run("struct-map-key-option-map-default", func(t *testing.T) {
		doTestSearchStructMapKeyOptions(t, nil)
	})
	t.Run("struct-map-key-option-map-string", func(t *testing.T) {
		doTestSearchStructMapKeyOptions(t, reflect.TypeOf(map[string]interface{}(nil)))
	})
}
func doTestSearchStructMapKeyOptions(t *testing.T, defaultMapType reflect.Type) {

	ctx := newSearchTestContext(t, defaultMapType)

	type testCase struct {
		mappy   interface{}
		opt     SearchOption
		want    []string
		wantErr bool
	}
	for i, tc := range []*testCase{
		{
			mappy: map[string]any{"mappy": map[searchStructKey]string{
				searchStructKey{Name: "zero"}:         "zero",
				searchStructKey{Name: "one", Age: 12}: "one"}},
			opt:     SearchOption{MapKeyFirst: true},
			wantErr: true},
		{
			mappy: map[string]any{"mappy": map[searchStructKey]string{
				searchStructKey{Name: "zero"}:         "zero",
				searchStructKey{Name: "one", Age: 12}: "one"}},
			opt: SearchOption{
				MapKeyAll: true,
				KeyStructStringer: func(keys []string, k any) (string, bool) {
					// if defaultMapType is map[interface{}]interface{}
					if m, ok := k.(map[interface{}]interface{}); ok {
						if n, ok := m["name"]; ok {
							return n.(string), true
						}
					}
					// if defaultMapType is map[string]interface{}
					if m, ok := k.(map[string]interface{}); ok {
						if n, ok := m["name"]; ok {
							return n.(string), true
						}
					}
					return "", false
				},
			},
			want: []string{"zero", "one"}},
		{
			mappy: map[string]any{"mappy": map[searchTaggedStructKey]string{
				searchTaggedStructKey{Name: "zero"}:         "zero",
				searchTaggedStructKey{Name: "one", Age: 12}: "one"}},
			opt: SearchOption{
				MapKeyAll: true,
				KeyStructStringer: func(keys []string, k any) (string, bool) {
					if m, ok := k.(searchTaggedStructKey); ok {
						return m.Name, true
					}
					return "", false
				},
			},
			want: []string{"zero", "one"}},
		{
			mappy: map[string]any{"mappy": map[searchTaggedStructKey]string{
				searchTaggedStructKey{Name: "zero"}:         "zero",
				searchTaggedStructKey{Name: "one", Age: 12}: "one"}},
			opt: SearchOption{
				MapKeyAll: true,
				KeyStructStringer: func(keys []string, k any) (string, bool) {
					return "", false
				},
			},
			wantErr: true},
	} {
		_, mbytes := ctx.mapBytes(tc.mappy)
		sea, err := ctx.dec.NewSearcher(bytes.NewReader(mbytes))
		if err != nil {
			t.Fatalf("unexpected %v", err)
		}

		info, err := sea.Search([]string{"mappy"}, tc.opt)
		if tc.wantErr {
			if err == nil {
				t.Errorf("case %d, expected error, but got nil", i)
			} else if len(err.Error()) == 0 {
				t.Errorf("case %d, empty error", i)
			}
			continue
		}
		if err != nil {
			t.Errorf("case %d, %v", i, err)
		}

		if len(info.MapKeys) != len(tc.want) {
			t.Fatalf("case %d, wrong length, expected: %v, got: %v", i, tc.want, info.MapKeys)
		}
		for j := 0; j < len(tc.want); j++ {
			if info.MapKeys[j] != tc.want[j] {
				t.Fatalf("case %d, unexpected key at %d, expected: %v, got: %v", i, j, tc.want, info.MapKeys)
			}
		}

		var val interface{}
		err = sea.SearchValue([]string{"mappy", info.MapKeys[0]}, &val, tc.opt)
		if err != nil {
			t.Errorf("case %d, %v", i, err)
		}
		if val != "zero" {
			t.Errorf("case %d, expected 'zero', got %v", i, val)
		}
	}
}

func TestSearchIndefiniteLength(t *testing.T) {
	encOpts := EncOptions{
		Sort:          SortCoreDeterministic,
		ShortestFloat: ShortestFloat16,
		NaNConvert:    NaNConvert7e00,
		InfConvert:    InfConvertFloat16,
	}
	encm, err := encOpts.EncMode()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	decm, err := DecOptions{DefaultMapType: reflect.TypeOf((map[string]interface{})(nil))}.DecMode()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var arrBytes []byte
	{
		buf := bytes.NewBuffer(make([]byte, 0))
		enc := encm.NewEncoder(buf)
		err := enc.StartIndefiniteArray()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		for _, s := range []string{"zero", "one", "two"} {
			err = enc.Encode(s)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		}
		err = enc.EndIndefinite()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		arrBytes = buf.Bytes()
	}

	var mapBytes []byte
	{
		buf := bytes.NewBuffer(make([]byte, 0))
		enc := encm.NewEncoder(buf)
		err := enc.StartIndefiniteMap()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		for k, v := range map[string]string{"one": "one", "two": "two", "three": "three"} {
			err = enc.Encode(k)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			err = enc.Encode(v)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		}
		err = enc.EndIndefinite()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		mapBytes = buf.Bytes()
	}

	type testCase struct {
		name    string
		bytes   []byte
		opt     SearchOption
		wantLen int
		lookup  []string
		wantVal string
	}

	for _, tc := range []*testCase{
		{name: "array", bytes: arrBytes, opt: SearchOption{Length: true}, wantLen: 3, lookup: []string{"0"}, wantVal: "zero"},
		{name: "map", bytes: mapBytes, opt: SearchOption{Length: true}, wantLen: 3, lookup: []string{"one"}, wantVal: "one"},
	} {

		t.Run(tc.name, func(t *testing.T) {
			sea, err := decm.NewSearcher(bytes.NewReader(tc.bytes))
			if err != nil {
				t.Fatalf("unexpected %v", err)
			}

			info, err := sea.Search([]string{}, tc.opt)
			if err != nil {
				t.Fatalf("unexpected %v", err)
			}
			if info.Length != tc.wantLen {
				t.Errorf("wrong length, expected: %d, got: %d", tc.wantLen, info.Length)
			}

			var val any
			err = sea.SearchValue(tc.lookup, &val)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if val != tc.wantVal {
				t.Errorf("expected: %s, got: %v", tc.wantVal, val)
			}

		})
	}
}

func TestNewSearcherError(t *testing.T) {
	ctx := newSearchTestContext(t)
	_, err := ctx.dec.NewSearcher(bytes.NewReader([]byte{}))
	if err == nil {
		t.Errorf("expected error, but got nil")
	}
}

func TestSkipError(t *testing.T) {
	ctx := newSearchTestContext(t)
	_, mbytes := ctx.mapBytes(searchMap())

	buf := bytes.NewBuffer(make([]byte, 0))
	_, err := buf.Write(mbytes)
	if err != nil {
		t.Errorf("unexpected %v", err)
	}
	sea, err := ctx.dec.NewSearcher(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Errorf("unexpected %v", err)
	}
	err = sea.Skip()
	if err == nil {
		t.Errorf("expected error, but got nil")
	}
	err = sea.Skip()
	if err == nil {
		t.Errorf("expected error, but got nil")
	}
}
