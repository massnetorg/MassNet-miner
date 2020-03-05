package logging

import (
	"os"
	"path/filepath"
	"time"

	rotatelogs "github.com/lestrrat/go-file-rotatelogs"
	"github.com/rifflock/lfshook"
	"github.com/sirupsen/logrus"
)

// NewFileRotateHooker enable log file output
func NewFileRotateHooker(path, filename string, age uint32, formatter logrus.Formatter) logrus.Hook {
	if len(path) == 0 {
		panic("Failed to parse logger folder:" + path + ".")
	}
	if !filepath.IsAbs(path) {
		path, _ = filepath.Abs(path)
	}
	if err := os.MkdirAll(path, 0700); err != nil {
		panic("Failed to create logger folder:" + path + ". err:" + err.Error())
	}
	filePath := filepath.Join(path, filename+"-%Y%m%d-%d.log")
	linkPath := filepath.Join(path, filename+".log")
	writer, err := rotatelogs.New(
		filePath,
		rotatelogs.WithLinkName(linkPath),
		// rotatelogs.WithRotationCount()
		rotatelogs.WithRotationTime(time.Duration(24)*time.Hour),
	)
	// set log file max age (age represents for year)
	if age > 0 {
		rotatelogs.WithMaxAge(time.Duration(age) * time.Duration(365) * time.Duration(24) * time.Hour).Configure(writer)
	}

	if err != nil {
		panic("Failed to create rotate logs. err:" + err.Error())
	}

	hook := lfshook.NewHook(lfshook.WriterMap{
		logrus.TraceLevel: writer,
		logrus.DebugLevel: writer,
		logrus.InfoLevel:  writer,
		logrus.WarnLevel:  writer,
		logrus.ErrorLevel: writer,
		logrus.FatalLevel: writer,
	}, formatter)
	return hook
}
