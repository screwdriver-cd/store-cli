package logger

import (
	"fmt"
	log "github.com/sirupsen/logrus"
	"os"
)

var logger = log.WithFields(log.Fields{"app": "store-cli"})

const (
	LOGLEVEL_ERROR = log.ErrorLevel
	LOGLEVEL_WARN  = log.WarnLevel
	LOGLEVEL_INFO  = log.InfoLevel
)

const (
	ERRTYPE_COPY         = "copy"
	ERRTYPE_SCOPE        = "scope"
	ERRTYPE_COMMAND      = "command"
	ERRTYPE_FILE         = "file"
	ERRTYPE_MD5          = "md5"
	ERRTYPE_ZIP          = "zip"
	ERRTYPE_MAXSIZELIMIT = "maxsizelimit"
)

func init() {
	log.SetFormatter(&log.JSONFormatter{PrettyPrint: true})
	log.SetOutput(os.Stdout)
	log.SetLevel(log.ErrorLevel)
}

/*
write log
param - level         		log level
param - errType
param - msg			error msg
return - error / nil		INFO / WARN - nil; ERROR - return error msg
*/
func Log(level log.Level, module, errType string, msg ...interface{}) error {
	switch level {
	case log.WarnLevel:
		msg = append([]interface{}{"ignore warning, "}, msg...)
		logger.WithFields(log.Fields{"module": module, "type": errType}).Warn(msg...)
		return nil
	case log.ErrorLevel:
		logger.WithFields(log.Fields{"module": module, "type": errType}).Error(msg...)
		return fmt.Errorf(fmt.Sprintf("%v", msg...))
	default:
		logger.WithFields(log.Fields{"module": module, "type": errType}).Info(msg...)
		return nil
	}
}
