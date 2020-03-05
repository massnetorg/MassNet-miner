package logging

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/sirupsen/logrus"
)

type functionHooker struct {
	innerLogger *Logger
	file        string
}

func (h *functionHooker) fire(entry *logrus.Entry) {
	pc, _, _, ok := runtime.Caller(7) // TODO: need match logrus
	if !ok {
		return
	}

	f := runtime.FuncForPC(pc)
	fname := f.Name()
	file, line := f.FileLine(pc)
	if strings.Contains(fname, "/") {
		index := strings.LastIndex(fname, "/")
		entry.Data["func"] = fname[index+1:]
	} else {
		entry.Data["func"] = fname
	}
	entry.Data["line"] = line
	entry.Data["file"] = filepath.Base(file)
}
func (h *functionHooker) fires(entry *logrus.Entry) {
	for i := 7; i < 10; i++ {
		pc, _, _, ok := runtime.Caller(i) // TODO: need match logrus
		if false == ok {
			break
		}
		f := runtime.FuncForPC(pc)
		file, line := f.FileLine(pc)
		fname := f.Name()
		var funcx string
		if strings.Contains(fname, "/") {
			index := strings.LastIndex(fname, "/")
			funcx = fname[index+1:]
		} else {
			funcx = fname
		}
		entry.Data["f"+strconv.Itoa(i)] = fmt.Sprintf("{%s,%s,%d}", filepath.Base(file), funcx, line)
	}
}

func (h *functionHooker) Fire(entry *logrus.Entry) error {
	if h.innerLogger.CallRelation == MsgFormatMulti {
		h.fires(entry)
	} else if h.innerLogger.CallRelation == MsgFormatSingle {
		h.fire(entry)
	}
	return nil
}

func (h *functionHooker) Levels() []logrus.Level {
	return []logrus.Level{
		logrus.PanicLevel,
		logrus.FatalLevel,
		logrus.ErrorLevel,
		logrus.WarnLevel,
		logrus.InfoLevel,
		logrus.DebugLevel,
		logrus.TraceLevel,
	}
}

// LoadFunctionHooker loads a function hooker to the logger
func LoadFunctionHooker(logger *Logger) {
	_, file, _, ok := runtime.Caller(1)
	if !ok {
		file = "unknown"
	}
	inst := &functionHooker{
		innerLogger: logger,
		file:        file,
	}
	logger.Hooks.Add(inst)
}
