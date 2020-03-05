/*
usage:

1. Read / Write

	var bm BucketMeta

	err = Update(db, func(tx DBTransaction) error {
		topBucket, err := tx.CreateTopLevelBucket("123")
		bucket := topBucket.NewBucket(name)
		if err != nil {
			return err //returning err will trigger rollback
		}
		...

		bm = bucket.GetBucketMeta()
		b := tx.FetchBucket(bm)

	return err
})


2. Only read
	err = View(db, func(tx DBTransaction) error {
		s, err := tx.BucketNames()
		assert.Equal(t, test.GetBucketName, s)
		return err
	})

*/

package db
