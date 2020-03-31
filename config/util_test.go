package config

import (
	"errors"
	"testing"
)

func TestSeed(t *testing.T) {
	tests := []struct {
		seed        string
		defaultPort string
		err         error
	}{
		{
			seed:        "192.168.1.100:43453",
			defaultPort: "43453",
			err:         nil,
		},
		{
			seed:        "[::2]",
			defaultPort: "43453",
			err:         nil,
		},
		{
			seed:        "massnet.org:43453",
			defaultPort: "43453",
			err:         nil,
		},
		{
			seed:        "192.168.1.100",
			defaultPort: "65536",
			err:         errors.New("invalid port 65536"),
		},
		{
			seed:        "192.168.1.100:65536",
			defaultPort: "43453",
			err:         errors.New("invalid port 65536"),
		},
	}

	for i, test := range tests {
		_, err := NormalizeSeed(test.seed, test.defaultPort)
		if err != nil && test.err != nil {
			if test.err.Error() != err.Error() {
				t.Errorf("%d, expect err is %s, got err is %s", i, test.err, err)
			}
		} else if test.err != err {
			t.Errorf("%d, expect err is %s, got err is %s", i, test.err, err)
		}
	}
}
