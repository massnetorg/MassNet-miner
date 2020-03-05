package logging

import (
	"sync/atomic"
	"testing"
)

func TestWarn(t *testing.T) {
	t.Run("log", func(t *testing.T) {
		Init("log", "test", "warn", 1, false)
		CPrint(WARN, "The group's number increased tremendously!",
			LogFormat{
				"omg":    true,
				"number": 122,
			})
		CPrint(ERROR, "A group of walrus emerges from the ocean",
			LogFormat{
				"animal": "walrus",
				"size":   10,
			})
		CPrint(ERROR, "A group of walrus emerges from the ocean", nil)

		//only in file
		VPrint(ERROR, "The group's number increased tremendously!",
			LogFormat{
				"omg":    true,
				"number": 122,
			})
		VPrint(WARN, "The group's number increased tremendously!",
			LogFormat{
				"omg":    true,
				"number": 122,
			})
		VPrint(WARN, "The group's number increased tremendously!", nil)
	})
}

func TestDebug(t *testing.T) {
	t.Run("log", func(t *testing.T) {
		Init("log", "test", "debug", 1, true)
		CPrint(TRACE, "The group's number increased tremendously!",
			LogFormat{
				"omg":    true,
				"number": 122,
			})
		CPrint(DEBUG, "A group of walrus emerges from the ocean",
			LogFormat{
				"animal": "walrus",
				"size":   10,
			})
		CPrint(ERROR, "A group of walrus emerges from the ocean", nil)

		//only in file
		VPrint(TRACE, "The group's number increased tremendously!",
			LogFormat{
				"omg":    true,
				"number": 122,
			})
		VPrint(WARN, "The group's number increased tremendously!",
			LogFormat{
				"omg":    true,
				"number": 122,
			})
		VPrint(WARN, "The group's number increased tremendously!", nil)
	})
}

func TestGid(t *testing.T) {
	t.Run("gid", func(t *testing.T) {
		Init("log", "test", "info", 1, false)
		var index int32 = 0
		chs := make([]chan int, 10)
		for i := 0; i < 10; i++ {
			chs[i] = make(chan int)
			go func(ch chan int) {
				atomic.AddInt32(&index, 1)
				CPrint(INFO, "The group's number increased tremendously!",
					LogFormat{
						"omg":    true,
						"number": 122,
						"index":  index,
					})
				ch <- 1
			}(chs[i])
		}
		for _, ch := range chs {
			<-ch
		}
	})
}

func TestFault(t *testing.T) {
	t.Run("fault", func(t *testing.T) {
		// Init("log", "warn", 1)
		// CPrint(FATAL, "The group's number increased tremendously!",
		// 	LogFormat{
		// 		"omg":    true,
		// 		"number": 122,
		// 	})
		// t.Errorf("is not exit")
	})
}
