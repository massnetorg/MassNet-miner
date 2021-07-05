package skchia

import (
	"errors"
	_ "massnet.org/mass/poc/engine.v2/massdb/massdb.chiapos"
	"regexp"
	"strings"
	"testing"
)

func TestMatchMassDBName(t *testing.T) {
	var regStr = `^PLOT-K\d{2}-\d{4}(-\d{2}){4}-[A-F0-9]{64}\.PLOT$`
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
			str:   "PLOT-K32-2021-04-16-08-06-C9F810532FA64861311FEDB29A10DADA4991E96F254693538B8D561084553201.PLOT",
			match: true,
		},
		{
			str:   "plot-k32-2021-04-16-08-06-c9f810532fa64861311fedb29a10dada4991e96f254693538b8d561084553201.plot",
			match: true,
		},
		{
			str:   ".plot-k32-2021-04-16-08-06-c9f810532fa64861311fedb29a10dada4991e96f254693538b8d561084553201.plot",
			match: false,
		},
		{
			str:   "plot-k32-2021-04-16-08-c9f810532fa64861311fedb29a10dada4991e96f254693538b8d561084553201.plot",
			match: false,
		},
		{
			str:   "plot-k32-2021-04-16-08-06-11-c9f810532fa64861311fedb29a10dada4991e96f254693538b8d561084553201.plot",
			match: false,
		},
		{
			str:   "plot-k-2021-04-16-08-06-c9f810532fa64861311fedb29a10dada4991e96f254693538b8d561084553201.plot",
			match: false,
		},
	}

	for i, test := range tests {
		if regExp.MatchString(strings.ToUpper(test.str)) != test.match {
			t.Error(i, ErrNotMatch)
		}
	}
}
