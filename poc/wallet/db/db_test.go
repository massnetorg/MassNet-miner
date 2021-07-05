package db_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	walletdb "massnet.org/mass/poc/wallet/db"

	"github.com/stretchr/testify/assert"
	_ "massnet.org/mass/poc/wallet/db/ldb"
	_ "massnet.org/mass/poc/wallet/db/rdb"
)

const testDbRoot = "testDbs"

var (
	dbtype          = ""
	triggerRollback = errors.New("trigger rollback")
)

// filesExists returns whether or not the named file or directory exists.
func fileExists(name string) bool {
	if _, err := os.Stat(name); err != nil {
		if os.IsNotExist(err) {
			return false
		}
	}
	return true
}

func GetDb(dbName string) (walletdb.DB, func(), error) {
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
	db, err := walletdb.CreateDB(dbtype, dbPath)
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

func TestAll(t *testing.T) {
	for _, tp := range walletdb.RegisteredDbTypes() {
		dbtype = tp
		t.Logf("run tests with %s...", tp)
		testDB_NewRootBucket(t)
		testDB_RootBucket(t)
		testDB_RootBucketNames(t)
		testDB_DeleteRootBucket(t)
		testBucket_NewSubBucket(t)
		testBucket_SubBucket(t)
		testBucket_SubBucketNames(t)
		testBucket_DeleteSubBucket(t)
		testBucket_Put(t)
		testBucket_Get(t)
		testBucket_Delete(t)
		testBucket_Clear(t)
		testCreateOrOpenDB(t)
		testGetByPrefix(t)
	}
}

//test create a new bucket
func testDB_NewRootBucket(t *testing.T) {
	tests := []struct {
		name               string
		dbName             string
		newRootBucketTimes int
		err                string
	}{
		{
			"valid",
			"123",
			1,
			"",
		}, {
			"duplicate bucket",
			"123",
			2,
			"bucket already exist",
		},
	}

	db, tearDown, err := GetDb("Tst_NewRootBucket")
	if err != nil {
		t.Errorf("init db error:%v", err)
	}
	defer tearDown()

	err = walletdb.Update(db, func(tx walletdb.DBTransaction) error {
		_, err := tx.CreateTopLevelBucket("123")
		if err != nil {
			t.Errorf("New RootBucket error:%v", err)
		}
		return err
	})
	assert.Nil(t, err)

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.newRootBucketTimes != 1 {

				_ = walletdb.Update(db, func(tx walletdb.DBTransaction) error {
					_, err := tx.CreateTopLevelBucket("123")
					if err != nil {
						assert.Equal(t, test.err, err.Error())
					}
					return err
				})
			}
		})
	}
}

func testDB_RootBucket(t *testing.T) {
	tests := []struct {
		name       string
		bucketName string
		err        string
	}{
		{
			"valid",
			"123",
			"",
		}, {
			"invalid bucket name",
			"",
			"invalid bucket name",
		},
	}
	db, tearDown, err := GetDb("Tst_RootBucket")
	if err != nil {
		t.Errorf("init db error:%v", err)
	}
	defer tearDown()
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_ = walletdb.Update(db, func(tx walletdb.DBTransaction) error {
				bucket := tx.TopLevelBucket("123")
				assert.Nil(t, bucket)
				return nil
			})
			// _, err = walletdb.Bucket(test.bucketName)
			// if err != nil {
			// 	assert.Equal(t, test.err, err.Error())
			// }
		})
	}
}

func testDB_RootBucketNames(t *testing.T) {
	tests := []struct {
		name          string
		bucketName    []string
		GetBucketName []string
	}{
		{
			"valid",
			[]string{"123", "456"},
			[]string{"123", "456"},
		},
	}
	db, tearDown, err := GetDb("Tst_RootBucketNames")
	if err != nil {
		t.Errorf("init db error:%v", err)
	}
	defer tearDown()
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for _, num := range test.bucketName {
				err = walletdb.Update(db, func(tx walletdb.DBTransaction) error {
					_, err := tx.CreateTopLevelBucket(num)
					return err
				})
				assert.Nil(t, err)
			}
			_ = walletdb.View(db, func(tx walletdb.ReadTransaction) error {
				s, err := tx.BucketNames()
				assert.Nil(t, err)
				assert.Equal(t, test.GetBucketName, s)
				return nil
			})
		})
	}
}

func testDB_DeleteRootBucket(t *testing.T) {
	tests := []struct {
		name       string
		rootBucket string
		error      string
	}{
		{
			"valid",
			"123",
			"not supported",
		},
	}
	db, tearDown, err := GetDb("Tst_DeleteRootBucket")
	if err != nil {
		t.Errorf("init db error:%v", err)
	}
	defer tearDown()
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_ = walletdb.Update(db, func(tx walletdb.DBTransaction) error {
				_, _ = tx.CreateTopLevelBucket(test.rootBucket)
				err := tx.DeleteTopLevelBucket(test.rootBucket)
				assert.Equal(t, test.error, err.Error())
				return nil
			})
		})
	}
}

func testBucket_NewSubBucket(t *testing.T) {
	tests := []struct {
		name              string
		dbName            string
		newSubBucketTimes int
		err               string
	}{
		{
			"valid",
			"123",
			1,
			"",
		}, {
			"duplicate bucket",
			"123",
			2,
			"bucket already exist",
		},
	}

	db, tearDown, err := GetDb("Tst_NewSubBucket")
	if err != nil {
		t.Errorf("init db error:%v", err)
	}
	defer tearDown()

	_ = walletdb.Update(db, func(tx walletdb.DBTransaction) error {
		bucket, err := tx.CreateTopLevelBucket("root")
		assert.Nil(t, err, fmt.Sprintf("New RootBucket error:%v", err))
		if err != nil {
			return err
		}

		bucket, err = bucket.NewBucket("123")
		assert.Nil(t, err)
		t.Logf("create sub bucket %v", bucket.GetBucketMeta().Paths())
		return err
	})

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.newSubBucketTimes != 1 {
				_ = walletdb.Update(db, func(tx walletdb.DBTransaction) error {
					bucket := tx.TopLevelBucket("root")
					assert.NotNil(t, bucket)
					_, err = bucket.NewBucket("123")
					assert.Equal(t, test.err, err.Error())
					return err
				})
			}
		})
	}
}

func testBucket_SubBucket(t *testing.T) {
	tests := []struct {
		name       string
		bucketName string
		err        string
	}{
		{
			"valid",
			"123",
			"",
		}, {
			"invalid bucket name",
			"",
			"invalid bucket name",
		},
	}

	db, tearDown, err := GetDb("Tst_SubBucket")
	if err != nil {
		t.Errorf("init db error:%v", err)
	}
	defer tearDown()

	_ = walletdb.Update(db, func(tx walletdb.DBTransaction) error {
		_, err := tx.CreateTopLevelBucket("abc")
		if err != nil {
			t.Errorf("New RootBucket error:%v", err)
			return err
		}
		return nil
	})

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {

			_ = walletdb.Update(db, func(tx walletdb.DBTransaction) error {
				bucket := tx.TopLevelBucket("abc")
				assert.NotNil(t, bucket)
				_, err = bucket.NewBucket(test.bucketName)
				if err != nil {
					assert.Equal(t, test.err, err.Error())
				}
				return nil
			})
		})
	}
}

func testBucket_SubBucketNames(t *testing.T) {
	tests := []struct {
		name          string
		bucketName    []string
		GetBucketName []string
	}{
		{
			"valid",
			[]string{"123", "456"},
			[]string{"123", "456"},
		},
	}
	db, tearDown, err := GetDb("Tst_SubBucketNames")
	if err != nil {
		t.Errorf("init db error:%v", err)
	}
	defer tearDown()

	_ = walletdb.Update(db, func(tx walletdb.DBTransaction) error {
		_, err := tx.CreateTopLevelBucket("abc")
		if err != nil {
			t.Errorf("New RootBucket error:%v", err)
			return err
		}
		return nil
	})

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {

			_ = walletdb.Update(db, func(tx walletdb.DBTransaction) error {
				bucket := tx.TopLevelBucket("abc")
				assert.NotNil(t, bucket)
				for _, num := range test.bucketName {
					_, err := bucket.NewBucket(num)
					assert.Nil(t, err)
				}
				return nil
			})

			_ = walletdb.View(db, func(tx walletdb.ReadTransaction) error {
				bucket := tx.TopLevelBucket("abc")
				assert.NotNil(t, bucket)
				s, _ := bucket.BucketNames()
				assert.Equal(t, test.GetBucketName, s)
				return nil
			})
		})
	}
}

func testBucket_DeleteSubBucket(t *testing.T) {
	tests := []struct {
		name              string
		bucketName        string
		bucketToDelete    string
		bucketAfterDelete []string
		err               string
	}{
		{
			"valid",
			"123",
			"123",
			[]string{},
			"",
		}, {
			"invalid bucket name",
			"123",
			"",
			[]string{"123"},
			"invalid bucket name",
		}, {
			"invalid bucket name",
			"123",
			"456",
			[]string{"123"},
			"",
		},
	}

	db, tearDown, err := GetDb("Tst_DeleteSubBucket")
	if err != nil {
		t.Errorf("init db error:%v", err)
	}
	defer tearDown()

	_ = walletdb.Update(db, func(tx walletdb.DBTransaction) error {
		_, err := tx.CreateTopLevelBucket("abc")
		if err != nil {
			t.Errorf("New RootBucket error:%v", err)
			return err
		}
		return nil
	})

	walletdb.View(db, func(tx walletdb.ReadTransaction) error {
		names, err := tx.BucketNames()
		assert.Nil(t, err)
		assert.Equal(t, []string{"abc"}, names)

		bucket := tx.TopLevelBucket("abc")
		names, err = bucket.BucketNames()
		assert.Nil(t, err)
		assert.Equal(t, []string{}, names)
		return nil
	})

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {

			_ = walletdb.Update(db, func(tx walletdb.DBTransaction) error {
				bucket := tx.TopLevelBucket("abc")
				assert.NotNil(t, bucket)

				_, err := bucket.NewBucket(test.bucketName)
				assert.Nil(t, err)
				s1, _ := bucket.BucketNames()
				assert.NotNil(t, s1)
				err = bucket.DeleteBucket(test.bucketToDelete)
				if err != nil {
					assert.Equal(t, test.err, err.Error())
				}
				bucket = tx.TopLevelBucket("abc")
				assert.NotNil(t, bucket)
				s, _ := bucket.BucketNames()
				assert.Equal(t, test.bucketAfterDelete, s)

				return triggerRollback
			})
		})
	}
}

func testBucket_Put(t *testing.T) {
	tests := []struct {
		name  string
		key   []byte
		value []byte
		err   string
	}{
		{
			"valid",
			[]byte{123},
			[]byte{123},
			"",
		}, {
			"illegal key",
			[]byte{},
			[]byte{123},
			"illegal key",
		}, {
			"illegal value",
			[]byte{123},
			[]byte{},
			"illegal value",
		},
	}
	db, tearDown, err := GetDb("Tst_Put")
	if err != nil {
		t.Errorf("init db error:%v", err)
	}
	defer tearDown()

	_ = walletdb.Update(db, func(tx walletdb.DBTransaction) error {
		_, err := tx.CreateTopLevelBucket("abc")
		if err != nil {
			t.Errorf("New RootBucket error:%v", err)
			return err
		}
		return nil
	})

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_ = walletdb.Update(db, func(tx walletdb.DBTransaction) error {
				bucket := tx.TopLevelBucket("abc")
				assert.NotNil(t, bucket)
				err := bucket.Put(test.key, test.value)
				if err != nil {
					assert.Equal(t, test.err, err.Error())
				} else {
					v, err := bucket.Get(test.key)
					assert.Nil(t, err)
					assert.Equal(t, test.value, v)
				}
				return triggerRollback
			})
		})
	}
}

func testBucket_Get(t *testing.T) {
	tests := []struct {
		name        string
		key         []byte
		value       []byte
		keyToFind   []byte
		valueToFind []byte
		err         string
	}{
		{
			"valid",
			[]byte{123},
			[]byte{123},
			[]byte{123},
			[]byte{123},
			"",
		}, {
			"invalid key",
			[]byte{123},
			[]byte{123},
			[]byte{},
			nil,
			"illegal key",
		}, {
			"key not found",
			[]byte{123},
			[]byte{123},
			[]byte{45},
			nil,
			"",
		},
	}
	db, tearDown, err := GetDb("Tst_Get")
	if err != nil {
		t.Errorf("init db error:%v", err)
	}
	defer tearDown()

	_ = walletdb.Update(db, func(tx walletdb.DBTransaction) error {
		_, err := tx.CreateTopLevelBucket("abc")
		if err != nil {
			t.Errorf("New RootBucket error:%v", err)
			return err
		}
		return nil
	})

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_ = walletdb.Update(db, func(tx walletdb.DBTransaction) error {
				bucket := tx.TopLevelBucket("abc")
				assert.NotNil(t, bucket)

				bucket.Put(test.key, test.value)
				value, err := bucket.Get(test.keyToFind)
				if err != nil {
					assert.Equal(t, test.err, err.Error())
				} else {
					assert.Equal(t, test.valueToFind, value)
				}
				return triggerRollback
			})
		})
	}
}

func testBucket_Delete(t *testing.T) {
	tests := []struct {
		name        string
		key         []byte
		value       []byte
		keyToDelete []byte
		valueToFind []byte
		err         string
	}{
		{
			"valid",
			[]byte{123},
			[]byte{123},
			[]byte{123},
			nil,
			"",
		}, {
			"invalid key",
			[]byte{123},
			[]byte{123},
			[]byte{},
			[]byte{123},
			"illegal key",
		}, {
			"key not found",
			[]byte{123},
			[]byte{123},
			[]byte{45},
			[]byte{123},
			"",
		},
	}
	db, tearDown, err := GetDb("Tst_Delete")
	if err != nil {
		t.Errorf("init db error:%v", err)
	}
	defer tearDown()

	_ = walletdb.Update(db, func(tx walletdb.DBTransaction) error {
		_, err := tx.CreateTopLevelBucket("abc")
		if err != nil {
			t.Errorf("New RootBucket error:%v", err)
			return err
		}
		return nil
	})

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_ = walletdb.Update(db, func(tx walletdb.DBTransaction) error {
				bucket := tx.TopLevelBucket("abc")
				assert.NotNil(t, bucket)
				bucket.Put(test.key, test.value)
				err := bucket.Delete(test.keyToDelete)
				if err != nil {
					assert.Equal(t, test.err, err.Error())
				}
				value, err := bucket.Get(test.key)
				assert.Nil(t, err)
				assert.Equal(t, test.valueToFind, value)
				return err
			})
		})
	}
}

func testBucket_Clear(t *testing.T) {
	tests := []struct {
		name        string
		key         []byte
		value       []byte
		keyToFind   []byte
		valueToFind []byte
		err         string
	}{
		{
			"valid",
			[]byte{123},
			[]byte{123},
			[]byte{123},
			nil,
			"",
		}, {
			"invalid key",
			[]byte{123},
			[]byte{123},
			[]byte{},
			nil,
			"illegal key",
		}, {
			"key not found",
			[]byte{123},
			[]byte{123},
			[]byte{45},
			nil,
			"",
		},
	}
	db, tearDown, err := GetDb("Tst_Clear")
	if err != nil {
		t.Errorf("init db error:%v", err)
	}
	defer tearDown()

	_ = walletdb.Update(db, func(tx walletdb.DBTransaction) error {
		_, err := tx.CreateTopLevelBucket("abc")
		if err != nil {
			t.Errorf("New RootBucket error:%v", err)
			return err
		}
		return nil
	})

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_ = walletdb.Update(db, func(tx walletdb.DBTransaction) error {
				bucket := tx.TopLevelBucket("abc")
				assert.NotNil(t, bucket)
				bucket.Put(test.key, test.value)
				err := bucket.Clear()
				if err != nil {
					assert.Equal(t, test.err, err.Error())
				}
				value, _ := bucket.Get(test.keyToFind)
				assert.Equal(t, test.valueToFind, value)
				return err
			})
		})
	}
}

func testCreateOrOpenDB(t *testing.T) {
	tests := []struct {
		name   string
		dbPath string
		create bool
		err    error
	}{
		{
			"create new db",
			testDbRoot + "/Tst_NewDB",
			true,
			nil,
		}, {
			"create existed db",
			testDbRoot + "/Tst_NewDB",
			true,
			walletdb.ErrCreateDBFailed,
		}, {
			"create invalid dbpath",
			"",
			true,
			walletdb.ErrCreateDBFailed,
		}, {
			"open existing db",
			testDbRoot + "/Tst_NewDB",
			false,
			nil,
		}, {
			"open non-existent db",
			testDbRoot + "/Tst_NonExistent",
			false,
			walletdb.ErrOpenDBFailed,
		}, {
			"open invalid dbpath",
			"",
			false,
			walletdb.ErrOpenDBFailed,
		},
	}

	if !fileExists(testDbRoot) {
		if err := os.MkdirAll(testDbRoot, 0700); err != nil {
			t.Errorf("unable to create test db root: %v", err)
			return
		}
	}
	defer func() {
		os.RemoveAll(testDbRoot)
	}()

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var (
				db  walletdb.DB
				err error
			)
			if test.create {
				db, err = walletdb.CreateDB(dbtype, test.dbPath)
			} else {
				db, err = walletdb.OpenDB(dbtype, test.dbPath)
			}

			assert.Equal(t, test.err, err)
			defer func() {
				if db != nil {
					db.Close()
				}
			}()
		})
	}
}

func testGetByPrefix(t *testing.T) {
	tests := []struct {
		name   string
		bucket string
		puts   map[string]string
		prefix string
		expect map[string]string
		err    error
	}{
		{
			"case 1",
			"case1",
			map[string]string{"a123": "123", "b456": "456", "c789": "789"},
			"d",
			map[string]string{},
			nil,
		},
		{
			"case 2",
			"case2",
			map[string]string{"a123": "123", "a223": "223", "b456": "456", "c789": "789"},
			"a",
			map[string]string{"a123": "123", "a223": "223"},
			nil,
		},
		{
			"case 3",
			"case3",
			map[string]string{"a123": "123", "a223": "223", "b456": "456", "c789": "789"},
			"a2",
			map[string]string{"a223": "223"},
			nil,
		},
		{
			"case 4",
			"case4",
			map[string]string{"a123": "123", "a223": "223", "b456": "456", "c789": "789"},
			"",
			map[string]string{"a123": "123", "a223": "223", "b456": "456", "c789": "789"},
			nil,
		},
		{
			"case 5",
			"case5",
			map[string]string{"a123": "123", "a1231": "1231", "b456": "456", "c789": "789"},
			"a123",
			map[string]string{"a123": "123", "a1231": "1231"},
			nil,
		},
	}

	db, tearDown, err := GetDb("Tst_GetByPrefix")
	if err != nil {
		t.Errorf("init db error:%v", err)
	}
	defer tearDown()

	_ = walletdb.Update(db, func(tx walletdb.DBTransaction) error {
		_, err := tx.CreateTopLevelBucket("root")
		if err != nil {
			t.Errorf("New RootBucket error:%v", err)
			return err
		}
		return nil
	})

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			walletdb.Update(db, func(tx walletdb.DBTransaction) error {
				root := tx.TopLevelBucket("root")
				assert.NotNil(t, root)
				bucket, err := root.NewBucket(test.bucket)
				if err != nil {
					t.FailNow()
				}
				for key, value := range test.puts {
					err := bucket.Put([]byte(key), []byte(value))
					assert.Nil(t, err)
				}
				return nil
			})

			walletdb.View(db, func(tx walletdb.ReadTransaction) error {
				root := tx.TopLevelBucket("root")
				assert.NotNil(t, root)
				bucket := root.Bucket(test.bucket)
				assert.NotNil(t, bucket)

				items, err := bucket.GetByPrefix([]byte(test.prefix))
				assert.Equal(t, test.err, err)
				if err != nil {
					return nil
				}

				assert.Equal(t, len(items), len(test.expect))
				for _, entry := range items {
					assert.Equal(t, test.puts[string(entry.Key)], string(entry.Value))
					fmt.Println("equal", string(entry.Key), string(entry.Value))
				}
				return nil
			})
		})
	}
}
