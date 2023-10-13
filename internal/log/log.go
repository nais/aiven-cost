package log

import (
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
)

func New(format, level string) (*logrus.Logger, error) {
	log := logrus.StandardLogger()

	switch format {
	case "json":
		log.SetFormatter(&logrus.JSONFormatter{
			TimestampFormat: time.RFC3339Nano,
		})
	case "text":
		log.SetFormatter(&logrus.TextFormatter{
			TimestampFormat: time.RFC3339Nano,
		})
	default:
		return nil, fmt.Errorf("invalid log format: %s", format)
	}

	lvl, err := logrus.ParseLevel(level)
	if err != nil {
		return nil, err
	}

	log.SetLevel(lvl)

	return log, nil
}
