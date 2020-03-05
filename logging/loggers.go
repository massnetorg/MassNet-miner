package logging

import (
	"bytes"
	"os"
	"runtime"
	"strconv"

	"github.com/sirupsen/logrus"
)

// const
const (
	PanicLevel = "panic"
	FatalLevel = "fatal"
	ErrorLevel = "error"
	WarnLevel  = "warn"
	InfoLevel  = "info"
	DebugLevel = "debug"
	TraceLevel = "trace"
)
const (
	//PANIC log level
	PANIC uint32 = iota
	//FATAL has list msg
	FATAL
	//ERROR has list msg
	ERROR
	//WARN only log
	WARN
	//INFO only log
	INFO
	//DEBUG only log
	DEBUG
	//TRACE only log
	TRACE
)
const (
	//MsgFormatSingle use info
	MsgFormatSingle uint32 = iota
	//MsgFormatMulti use show all func call relation
	MsgFormatMulti
)

// LogFormat is to log format
type LogFormat = map[string]interface{}

type emptyWriter struct{}

func (ew emptyWriter) Write(p []byte) (int, error) {
	return 0, nil
}

type Logger struct {
	*logrus.Logger
	//CallRelation to show stack list
	CallRelation uint32
}

func NewLogger() *Logger {
	return &Logger{
		Logger: logrus.New(),
	}
}

// SetCallList to set CallList
func (logger *Logger) SetCallRelation(button uint32) {
	logger.CallRelation = button
}

// logger pointer must be initialized, else would panic.
var clog *Logger
var vlog *Logger

func convertLevel(level string) logrus.Level {
	switch level {
	case PanicLevel:
		return logrus.PanicLevel
	case FatalLevel:
		return logrus.FatalLevel
	case ErrorLevel:
		return logrus.ErrorLevel
	case WarnLevel:
		return logrus.WarnLevel
	case InfoLevel:
		return logrus.InfoLevel
	case DebugLevel:
		return logrus.DebugLevel
	case TraceLevel:
		return logrus.TraceLevel
	default:
		return logrus.InfoLevel
	}
}

// Init loggers
func Init(path, filename string, level string, age uint32, disableCPrint bool) {
	fileHooker := NewFileRotateHooker(path, filename, age, nil)

	vlog = NewLogger()
	LoadFunctionHooker(vlog)
	vlog.Hooks.Add(fileHooker)
	vlog.Out = &emptyWriter{}
	vlog.Formatter = &logrus.TextFormatter{FullTimestamp: true}
	vlog.Level = convertLevel(level)

	if !disableCPrint {
		clog = NewLogger()
		LoadFunctionHooker(clog)
		clog.Hooks.Add(fileHooker)
		clog.Out = os.Stdout
		clog.Formatter = &logrus.TextFormatter{FullTimestamp: true}
		clog.Level = convertLevel(level)
	} else {
		clog = vlog
	}

	vlog.WithFields(logrus.Fields{
		"path":  path,
		"level": level,
	}).Info("Logger Configuration.")
}

// GetGID return gid
func GetGID() uint64 {
	b := make([]byte, 64)
	b = b[:runtime.Stack(b, false)]
	b = bytes.TrimPrefix(b, []byte("goroutine "))
	b = b[:bytes.IndexByte(b, ' ')]
	n, _ := strconv.ParseUint(string(b), 10, 64)
	return n
}

// CPrint into stdout + log
func CPrint(level uint32, msg string, formats ...LogFormat) {
	if clog == nil {
		Init("/tmp", "tmp-mass.log", "info", 0, false)
	}
	data := mergeLogFormats(formats...)
	switch level {
	case PANIC:
		{
			clog.SetCallRelation(MsgFormatMulti)
			clog.WithFields(data).Panic(msg)
			break
		}
	case FATAL:
		{
			clog.SetCallRelation(MsgFormatMulti)
			clog.WithFields(data).Fatal(msg)
			break
		}
	case ERROR:
		{
			clog.SetCallRelation(MsgFormatMulti)
			clog.WithFields(data).Error(msg)
			break
		}
	case WARN:
		{
			clog.SetCallRelation(MsgFormatSingle)
			clog.WithFields(data).Warn(msg)
			break
		}
	case INFO:
		{
			clog.SetCallRelation(MsgFormatSingle)
			clog.WithFields(data).Info(msg)
			break
		}
	case DEBUG:
		{
			clog.SetCallRelation(MsgFormatSingle)
			clog.WithFields(data).Debug(msg)
			break
		}
	case TRACE:
		{
			clog.SetCallRelation(MsgFormatSingle)
			clog.WithFields(data).Trace(msg)
			break
		}
	default:
		{
			clog.SetCallRelation(MsgFormatMulti)
			clog.WithFields(data).Error(msg)
		}
	}
}

// VPrint into log
func VPrint(level uint32, msg string, formats ...LogFormat) {
	if vlog == nil {
		Init("/tmp", "tmp-mass.log", "info", 0, false)
	}
	data := mergeLogFormats(formats...)
	switch level {
	case PANIC:
		{
			vlog.SetCallRelation(MsgFormatMulti)
			vlog.WithFields(data).Panic(msg)
			break
		}
	case FATAL:
		{
			vlog.SetCallRelation(MsgFormatMulti)
			vlog.WithFields(data).Fatal(msg)
			break
		}
	case ERROR:
		{
			vlog.SetCallRelation(MsgFormatMulti)
			vlog.WithFields(data).Error(msg)
			break
		}
	case WARN:
		{
			vlog.SetCallRelation(MsgFormatSingle)
			vlog.WithFields(data).Warn(msg)
			break
		}
	case INFO:
		{
			vlog.SetCallRelation(MsgFormatSingle)
			vlog.WithFields(data).Info(msg)
			break
		}
	case DEBUG:
		{
			vlog.SetCallRelation(MsgFormatSingle)
			vlog.WithFields(data).Debug(msg)
			break
		}
	case TRACE:
		{
			vlog.SetCallRelation(MsgFormatSingle)
			vlog.WithFields(data).Trace(msg)
			break
		}
	default:
		{
			vlog.SetCallRelation(MsgFormatMulti)
			vlog.WithFields(data).Error(msg)
		}
	}
}

// mergeLogFormats merges LogFormats.
// Same key would be covered by later-presented values.
func mergeLogFormats(formats ...LogFormat) LogFormat {
	format := LogFormat{}
	for _, data := range formats {
		if data == nil {
			continue
		}
		for k, v := range data {
			vv := v
			format[k] = vv
		}
	}
	format["tid"] = GetGID()
	return format
}
