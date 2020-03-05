package mock

import (
	"testing"
)

func TestInit(t *testing.T) {
	initTemplateData(20)
	for i := 0; i < 20; i++ {
		t.Log(basicBlocks[i])
	}

	for _, key := range pocKeys {
		t.Log(key)
	}
}
