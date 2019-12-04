package logger

import (
	log "github.com/sirupsen/logrus"
	"os"
)

func init() {
	log.SetFormatter(&log.JSONFormatter{})
	log.SetOutput(os.Stdout)
	log.SetLevel(log.InfoLevel)
}

var logger = log.WithFields(log.Fields{"app": "store-cli"})

const (
	WARN         = "warn"
	COPY         = "copy"
	SCOPE        = "scope"
	COMMAND      = "command"
	MAXSIZELIMIT = "maxsizelimit"
	INFO         = "info"
	ERROR        = "error"
	FILE         = "file"
	MD5          = "md5"
	ZIP          = "zip"
)

func Write(level, module, errType string, v ...interface{}) {
	switch level {
	case "info":
		logger.WithFields(log.Fields{"module": module, "type": errType}).Info(v...)
	case "warn":
		logger.WithFields(log.Fields{"module": module, "type": errType}).Warn(v...)
	case "error":
		logger.WithFields(log.Fields{"module": module, "type": errType}).Error(v...)
	}
}
