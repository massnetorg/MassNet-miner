package testutil

func SameErrorString(err, target error) bool {
	if err == nil && target == nil {
		return true
	}
	if err == nil || target == nil {
		return false
	}
	return err.Error() == target.Error()
}
