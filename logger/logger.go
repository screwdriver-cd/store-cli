package logger

import (
	"fmt"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const Loglevel = zap.ErrorLevel

var zapLogger *zap.Logger

func NewProductionEncoderConfig() zapcore.EncoderConfig {
	return zapcore.EncoderConfig{
		TimeKey:        "", // disable printing timestamp "ts"
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		MessageKey:     "msg",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.CapitalLevelEncoder,
		EncodeTime:     zapcore.RFC3339NanoTimeEncoder,
		EncodeDuration: zapcore.SecondsDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}
}

func init() {
	cfg := zap.NewProductionConfig()
	cfg.Encoding = "json"
	cfg.Level = zap.NewAtomicLevelAt(Loglevel)
	cfg.InitialFields = map[string]interface{}{"app": "store-cli"}
	cfg.OutputPaths = []string{"stdout"}
	cfg.ErrorOutputPaths = []string{"stderr"}
	cfg.DisableStacktrace = true
	cfg.DisableCaller = true
	cfg.EncoderConfig = NewProductionEncoderConfig()
	zapLogger, _ = cfg.Build()
	defer func() { _ = zapLogger.Sync() }()
}

func Info(msg string) {
	zapLogger.Info(msg)
}

func Warn(err ...interface{}) {
	msg := append([]interface{}{"IGNORE,"}, err...)
	zapLogger.Warn(fmt.Sprintf("%v", msg))
}

func Error(err error) error {
	zapLogger.Error(fmt.Sprintf("%v", err))
	return fmt.Errorf(fmt.Sprintf("%v", err))
}
