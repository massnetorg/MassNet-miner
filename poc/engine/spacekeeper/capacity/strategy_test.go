package capacity

import (
	"errors"
	"regexp"
	"strings"
	"testing"
)

func TestMatchMassDBName(t *testing.T) {
	var regStr = `^[A-F0-9]{66}-\d{2}-B\.MASSDB$`
	regExp, err := regexp.Compile(regStr)
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	var ErrNotMatch = errors.New("not match")
	tests := []struct {
		str   string
		match bool
	}{
		{
			str:   "88e55973485fd0b047c2e4fe682d9c9bb3e2c50e8c123d170c33b52c024932b4ab-10-a.massdb",
			match: false,
		},
		{
			str:   "88e55973485fd0b047c2e4fe682d9c9bb3e2c50e8c123d170c33b52c024932b4ab-10-b.massdb",
			match: true,
		},
		{
			str:   "88e55973485fd0b047c2e4fe682d9c9bb3e2c50e8c123d170c33b52c024932b4ab-10-bxmassdb",
			match: false,
		},
		{
			str:   "88e55973485fd0b047c2e4fe682d9c9bb3e2c50e8c123d170c33b52c024932b4ab-10-b1.massdb",
			match: false,
		},
		{
			str:   "088e55973485fd0b047c2e4fe682d9c9bb3e2c50e8c123d170c33b52c024932b4ab-10-b.massdb",
			match: false,
		},
		{
			str:   "8e55973485fd0b047c2e4fe682d9c9bb3e2c50e8c123d170c33b52c024932b4ab-10-b.massdb",
			match: false,
		},
	}

	for i, test := range tests {
		if regExp.MatchString(strings.ToUpper(test.str)) != test.match {
			t.Error(i, ErrNotMatch)
		}
	}
}
