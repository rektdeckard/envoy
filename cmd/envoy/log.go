package main

import (
	"os"

	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var log *zap.SugaredLogger

func initLogger(cmd *cobra.Command) {
	atom := zap.NewAtomicLevel()
	encoderCfg := zap.NewProductionEncoderConfig()
	unsugared := zap.New(zapcore.NewCore(
		zapcore.NewJSONEncoder(encoderCfg),
		zapcore.Lock(os.Stderr),
		atom,
	))
	defer unsugared.Sync()
	log = unsugared.Sugar()

	if logLevel, err := cmd.Flags().GetString("log-level"); err != nil {
		log.Fatalf("could not read log-level: %v", err)
	} else {
		if zapLevel, err := zap.ParseAtomicLevel(logLevel); err != nil {
			log.Fatalf("invalid log-level: %v", err)
		} else {
			atom.SetLevel(zapLevel.Level())
		}
	}

}
