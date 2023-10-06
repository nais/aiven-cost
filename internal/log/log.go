package log

import (
	"fmt"

	"golang.org/x/exp/slog"
)

func Infof(format string, args ...any) {
	slog.Default().Info(fmt.Sprintf(format, args...))
}

func Errorf(err error, format string, args ...any) {
	slog.Default().Error(fmt.Sprintf(format, args...), err)
}

func Warnf(format string, args ...any) {
	slog.Default().Warn(fmt.Sprintf(format, args...))
}
