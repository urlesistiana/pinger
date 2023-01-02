package mlog

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"os"
)

var (
	stderr = zapcore.Lock(os.Stderr)
	lvl    = zap.NewAtomicLevelAt(zap.InfoLevel)
	l      = zap.New(zapcore.NewCore(zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()), stderr, lvl))
)

func L() *zap.Logger {
	return l
}

func SetLevel(l zapcore.Level) {
	lvl.SetLevel(l)
}
