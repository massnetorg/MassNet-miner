package storage_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"massnet.org/mass/database"
	"massnet.org/mass/database/storage"
	_ "massnet.org/mass/database/storage/ldbstorage"
)

const testDbRoot = "testDbs"

var dbtype = ""

func TestAll(t *testing.T) {
	for _, tp := range storage.RegisteredDbTypes() {
		t.Logf("run tests with %s...", tp)
		dbtype = tp
		testPutGet(t)
		testDelete(t)
		testBatch(t)
		testIterator(t)
		testOverwrite(t)
		testSeek(t)
	}
}

// filesExists returns whether or not the named file or directory exists.
func fileExists(name string) bool {
	if _, err := os.Stat(name); err != nil {
		if os.IsNotExist(err) {
			return false
		}
	}
	return true
}

func GetStorage(dbName string) (storage.Storage, func(), error) {
	// Create the root directory for test databases.
	if !fileExists(testDbRoot) {
		if err := os.MkdirAll(testDbRoot, 0700); err != nil {
			err := fmt.Errorf("unable to create test db "+
				"root: %v", err)
			return nil, nil, err
		}
	}
	dbPath := filepath.Join(testDbRoot, dbName)
	if err := os.RemoveAll(dbPath); err != nil {
		err := fmt.Errorf("cannot remove old db: %v", err)
		return nil, nil, err
	}
	db, err := storage.CreateStorage(dbtype, dbPath, nil)
	if err != nil {
		fmt.Println("create db error: ", err)
		return nil, nil, err
	}
	tearDown := func() {
		dbVersionPath := filepath.Join(testDbRoot, dbName+".ver")
		db.Close()
		os.RemoveAll(dbPath)
		os.Remove(dbVersionPath)
		os.RemoveAll(testDbRoot)
	}
	return db, tearDown, nil
}

func testPutGet(t *testing.T) {
	putTests := []struct {
		name string
		puts map[string]string
		err  error
	}{
		{
			"normal",
			map[string]string{
				"a123": "a123",
				"b123": "b123",
			},
			nil,
		},
		{
			"empty key",
			map[string]string{
				"": "value",
			},
			storage.ErrInvalidKey,
		},
		{
			"empty value",
			map[string]string{
				"novalue": "",
			},
			nil,
		},
	}

	getTests := []struct {
		name   string
		key    string
		expect string
		err    error
	}{
		{
			"existing1",
			"a123",
			"a123",
			nil,
		},
		{
			"existing2",
			"b123",
			"b123",
			nil,
		},
		{
			"non-existent",
			"a",
			"",
			storage.ErrNotFound,
		},
		{
			"empty key",
			"",
			"",
			storage.ErrNotFound,
		},
		{
			"empty value",
			"novalue",
			"",
			nil,
		},
	}

	hasTests := []struct {
		name   string
		key    string
		expect bool
	}{
		{
			"existing1",
			"a123",
			true,
		},
		{
			"existing2",
			"b123",
			true,
		},
		{
			"non-existent",
			"a",
			false,
		},
		{
			"empty key",
			"",
			false,
		},
	}

	store, tearDown, err := GetStorage("Tst_PutGetHas")
	if err != nil {
		t.Errorf("init db error:%v", err)
		t.FailNow()
	}
	defer tearDown()

	for _, test := range putTests {
		t.Run(test.name, func(t *testing.T) {
			for key, value := range test.puts {
				err := store.Put([]byte(key), []byte(value))
				assert.Equal(t, test.err, err)
			}
		})
	}
	for _, test := range getTests {
		t.Run(test.name, func(t *testing.T) {
			actual, err := store.Get([]byte(test.key))
			assert.Equal(t, test.expect, string(actual))
			assert.Equal(t, test.err, err)
		})
	}
	for _, test := range hasTests {
		t.Run(test.name, func(t *testing.T) {
			actual, err := store.Has([]byte(test.key))
			assert.Equal(t, test.expect, actual)
			assert.Nil(t, err)
		})
	}
}

func testDelete(t *testing.T) {
	puts := map[string]string{
		"a123": "a123",
		"b456": "b456",
		"c789": "c789",
	}

	tests := []struct {
		name string
		key  string
		err  error
	}{
		{
			"empty key",
			"",
			nil,
		},
		{
			"non-existent key",
			"aaaa",
			nil,
		},
		{
			"existing key",
			"b456",
			nil,
		},
	}

	store, tearDown, err := GetStorage("Tst_Delete")
	if err != nil {
		t.Errorf("init db error:%v", err)
		t.FailNow()
	}
	defer tearDown()

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {

			for key, value := range puts {
				err := store.Put([]byte(key), []byte(value))
				assert.Nil(t, err)
			}

			before, _ := store.Has([]byte(test.key))
			err = store.Delete([]byte(test.key))
			assert.Equal(t, test.err, err)
			after, _ := store.Has([]byte(test.key))
			assert.False(t, before && after)
		})
	}
}

func testBatch(t *testing.T) {
	type op struct {
		put   bool // false->delete
		key   string
		value string
		err   error
	}

	tests := []struct {
		name           string
		ops            []op
		clear          bool
		expectExist    map[string]string
		expectNotExist map[string]string
		err            error
	}{
		{
			"no data",
			nil,
			false,
			nil,
			nil,
			nil,
		},
		{
			"clear",
			[]op{
				{
					true, // put
					"a123",
					"a123",
					nil,
				},
				{
					true, // put
					"b456",
					"b456",
					nil,
				},
			},
			true,
			nil,
			map[string]string{
				"a123": "a123",
				"b456": "b456",
			},
			nil,
		},
		{
			"normal",
			[]op{
				{
					true, // put
					"a123",
					"a123",
					nil,
				},
				{
					true, // put
					"b456",
					"b456",
					nil,
				},
				{
					false, // delete
					"a123",
					"a123",
					nil,
				},
				{
					true, // put
					"c789",
					"c789",
					nil,
				},
			},
			false,
			map[string]string{
				"b456": "b456",
				"c789": "c789",
			},
			map[string]string{
				"a123": "a123",
			},
			nil,
		},
	}

	store, tearDown, err := GetStorage("Tst_Batch")
	if err != nil {
		t.Errorf("init db error:%v", err)
		t.FailNow()
	}
	defer tearDown()

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			batch := store.NewBatch()
			defer batch.Release()

			for _, op := range test.ops {
				var err error
				if op.put {
					err = batch.Put([]byte(op.key), []byte(op.value))
				} else {
					err = batch.Delete([]byte(op.key))
				}
				assert.Equal(t, op.err, err)
			}
			if test.clear {
				batch.Reset()
			}
			err := store.Write(batch)
			assert.Equal(t, test.err, err)

			for k, v := range test.expectExist {
				ac, err := store.Get([]byte(k))
				assert.Nil(t, err)
				assert.Equal(t, v, string(ac))
			}

			for k := range test.expectNotExist {
				exist, err := store.Has([]byte(k))
				assert.Nil(t, err)
				assert.False(t, exist)
			}
		})
	}
}

func testIterator(t *testing.T) {

	puts := map[string]string{
		"a":      "a",
		"a1":     "a1",
		"a123":   "a123",
		"b123":   "b123",
		"b2":     "b2",
		"b4":     "b4",
		"b45":    "b45",
		"b45123": "b45123",
		"b456":   "b456",
		"b6":     "b6",
		"b756":   "b756",
		"c789":   "c789",
	}

	tests := []struct {
		name   string
		slice  *storage.Range
		expect map[string]string
		err    error
	}{
		{
			"nil range",
			nil,
			puts,
			nil,
		},
		{
			"by prefix - empty",
			storage.BytesPrefix([]byte("")),
			puts,
			nil,
		},
		{
			"by prefix - no match",
			storage.BytesPrefix([]byte("ddd")),
			nil,
			nil,
		},
		{
			"invalid range",
			&storage.Range{Start: []byte("b451233"), Limit: []byte("b451233")},
			nil,
			nil,
		},
		{
			"by prefix - normal",
			storage.BytesPrefix([]byte("b4")),
			map[string]string{
				"b45":    "b45",
				"b45123": "b45123",
				"b456":   "b456",
				"b4":     "b4",
			},
			nil,
		},
		{
			"by range - no item",
			&storage.Range{Start: []byte("b3"), Limit: []byte("b4")},
			nil,
			nil,
		},
		{
			"by range - 1",
			&storage.Range{Start: []byte("b3"), Limit: []byte("b40")},
			map[string]string{
				"b4": "b4",
			},
			nil,
		},
		{
			"by range - 2",
			&storage.Range{Start: []byte("b4"), Limit: []byte("b40")},
			map[string]string{
				"b4": "b4",
			},
			nil,
		},
		{
			"by range - 3",
			&storage.Range{Start: []byte("b2"), Limit: []byte("b4")},
			map[string]string{
				"b2": "b2",
			},
			nil,
		},
		{
			"by range - 4",
			&storage.Range{Start: []byte("b"), Limit: []byte("b5")},
			map[string]string{
				"b123":   "b123",
				"b2":     "b2",
				"b4":     "b4",
				"b45":    "b45",
				"b45123": "b45123",
				"b456":   "b456",
			},
			nil,
		},
		{
			"by range - 5",
			&storage.Range{Start: []byte("b451233"), Limit: []byte("b5")},
			map[string]string{
				"b456": "b456",
			},
			nil,
		},
	}

	store, tearDown, err := GetStorage("Tst_Iterator")
	if err != nil {
		t.Errorf("init db error:%v", err)
		t.FailNow()
	}
	defer tearDown()

	for k, v := range puts {
		err := store.Put([]byte(k), []byte(v))
		assert.Nil(t, err)
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mp := make(map[string]string)
			iter := store.NewIterator(test.slice)
			defer iter.Release()
			for iter.Next() {
				mp[string(iter.Key())] = string(iter.Value())
			}
			assert.Nil(t, iter.Error())

			assert.Equal(t, len(test.expect), len(mp))

			for k, v := range test.expect {
				actualV, ok := mp[k]
				assert.True(t, ok)
				assert.Equal(t, v, actualV)
			}
		})
	}
}

func testOverwrite(t *testing.T) {
	store, tearDown, err := GetStorage("Tst_Iterator")
	if err != nil {
		t.Errorf("init db error:%v", err)
		t.FailNow()
	}
	defer tearDown()

	key := []byte("key")
	oldValue := []byte("old value")
	newValue := []byte("new value")

	err = store.Put(key, oldValue)
	assert.Nil(t, err)
	v, err := store.Get(key)
	assert.Nil(t, err)
	assert.Equal(t, oldValue, v)

	// overwrite
	assert.Nil(t, store.Put(key, newValue))
	v, err = store.Get(key)
	assert.Nil(t, err)
	assert.Equal(t, newValue, v)
}

func TestCheckVersion(t *testing.T) {
	dbdir := "testDb"
	dbtype := "testdb"

	tests := []struct {
		name   string
		dbtype string
		dbpath string
		err    error
	}{
		{
			"normal",
			dbtype,
			dbdir,
			nil,
		},
		{
			"incorrect dbpath",
			dbtype,
			"wrongpath",
			database.ErrDbDoesNotExist,
		},
		{
			"incorrect dbtype",
			"wrongtype",
			dbdir,
			storage.ErrUnsupportedVersion,
		},
	}

	err := os.MkdirAll(dbdir, 0700)
	assert.Nil(t, err)
	defer func() {
		err = os.RemoveAll(dbdir)
		assert.Nil(t, err)
	}()
	// create
	err = storage.CheckVersion(dbtype, dbdir, true)
	assert.Nil(t, err)

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err = storage.CheckVersion(test.dbtype, test.dbpath, false)
			assert.Equal(t, test.err, err)
		})
	}
}

func testSeek(t *testing.T) {
	puts := map[string]string{
		"a":      "a",
		"a1":     "a1",
		"a123":   "a123",
		"b123":   "b123",
		"b2":     "b2",
		"b4":     "b4",
		"b45":    "b45",
		"b45123": "b45123",
		"b456":   "b456",
		"b6":     "b6",
		"b756":   "b756",
		"c789":   "c789",
	}

	type seekData struct {
		key    string
		expect bool
	}

	tests := []struct {
		name   string
		rg     *storage.Range
		seek   *seekData
		expect map[string]string
		err    error
	}{
		{
			name:   "case 1",
			rg:     nil,
			seek:   nil, // no seek
			expect: puts,
			err:    nil,
		},
		{
			name:   "case 2",
			rg:     &storage.Range{Start: []byte(""), Limit: []byte("")},
			seek:   nil, // no seek
			expect: puts,
			err:    nil,
		},
		{
			name: "case 3",
			rg:   &storage.Range{Start: []byte("b"), Limit: []byte("c")},
			seek: nil,
			expect: map[string]string{
				"b123":   "b123",
				"b2":     "b2",
				"b4":     "b4",
				"b45":    "b45",
				"b45123": "b45123",
				"b456":   "b456",
				"b6":     "b6",
				"b756":   "b756",
			},
			err: nil,
		},
		{
			name: "case 4",
			rg:   &storage.Range{Start: []byte("b"), Limit: []byte("c")},
			seek: &seekData{
				key:    "b2",
				expect: true,
			},
			expect: map[string]string{
				"b2":     "b2",
				"b4":     "b4",
				"b45":    "b45",
				"b45123": "b45123",
				"b456":   "b456",
				"b6":     "b6",
				"b756":   "b756",
			},
			err: nil,
		},
		{
			name: "case 5",
			rg:   &storage.Range{Start: []byte("b"), Limit: nil},
			seek: &seekData{
				key:    "b45",
				expect: true,
			},
			expect: map[string]string{
				"b45":    "b45",
				"b45123": "b45123",
				"b456":   "b456",
				"b6":     "b6",
				"b756":   "b756",
				"c789":   "c789",
			},
			err: nil,
		},
		{
			name: "case 6",
			rg:   &storage.Range{Start: []byte("b"), Limit: []byte("c")},
			seek: &seekData{
				key:    "c",
				expect: false,
			},
			expect: nil,
			err:    nil,
		},
	}

	store, tearDown, err := GetStorage("Tst_Iterator")
	if err != nil {
		t.Errorf("init db error:%v", err)
		t.FailNow()
	}
	defer tearDown()

	for k, v := range puts {
		err := store.Put([]byte(k), []byte(v))
		assert.Nil(t, err)
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			it := store.NewIterator(test.rg)
			defer it.Release()

			sought := false
			if test.seek != nil {
				sought = it.Seek([]byte(test.seek.key))
				assert.Equal(t, test.seek.expect, sought)
			}

			actual := make(map[string]string)
			if sought {
				actual[string(it.Key())] = string(it.Value())
			}
			for it.Next() {
				actual[string(it.Key())] = string(it.Value())
			}

			assert.Equal(t, len(test.expect), len(actual))
			for ek, ev := range test.expect {
				av, ok := actual[ek]
				assert.True(t, ok && ev == av, fmt.Sprintf("expect %s:%s, actual %s", ek, ev, av))
			}
		})
	}
}
