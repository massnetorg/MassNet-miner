package db

// GetOrCreateBucket return a sub store space(sub Bucket). Param db can also be a Bucket instance
func GetOrCreateBucket(bucket Bucket, name string) (b Bucket, err error) {
	if bucket == nil {
		return nil, ErrInvalidArgument
	}

	if b = bucket.Bucket(name); b == nil {
		b, err = bucket.NewBucket(name)
	}
	return
}

// GetOrCreateTopLevelBucket ...
func GetOrCreateTopLevelBucket(tx DBTransaction, name string) (b Bucket, err error) {
	if tx == nil {
		return nil, ErrInvalidArgument
	}
	if b = tx.TopLevelBucket(name); b == nil {
		b, err = tx.CreateTopLevelBucket(name)
	}
	return
}
