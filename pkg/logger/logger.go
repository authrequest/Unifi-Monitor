package logger

import (
	"fmt"
	"os"
	"time"

	"github.com/rs/zerolog"
)

var log = zerolog.New(
	zerolog.ConsoleWriter{
		Out:        os.Stderr,
		TimeFormat: time.RFC3339,
		FormatLevel: func(i interface{}) string {
			return fmt.Sprintf("[%-6s]", i)
		},
	},
).Level(zerolog.TraceLevel).With().Timestamp().Caller().Logger()

// Expose logger methods
func Info() *zerolog.Event    { return log.Info() }
func Error() *zerolog.Event   { return log.Error() }
func Fatal() *zerolog.Event   { return log.Fatal() }
func Warning() *zerolog.Event { return log.Warn() }
