package runner3

import (
	"fmt"
	"log"

	"github.com/mattn/go-colorable"
)

// Loggers. Different colors for different things to stand out better. These are
// initialized in Configure(). Colorizing also helps makes fresher related logs stand
// out from the binary being run logs.
var (
	events coloredLogger //for file change, build, run.
	other  coloredLogger //verbose logging, etc.
	errs   coloredLogger //errors
)

// getColorCode returns an escaped color code sequence for use in log.Print... funcs.
// If an unknown color is given, default to white text since most terminals have a
// dark background.
//
// I will admit I do not understand exactly what the Sprintf calls are doing. I know
// it is an escape sequence, that is all.
func getColorCode(color string) string {
	switch color {
	case "black":
		return fmt.Sprintf("\033[%sm", "30")
	case "red":
		return fmt.Sprintf("\033[%sm", "31")
	case "green":
		return fmt.Sprintf("\033[%sm", "32")
	case "yellow":
		return fmt.Sprintf("\033[%sm", "33")
	case "blue":
		return fmt.Sprintf("\033[%sm", "34")
	case "magenta":
		return fmt.Sprintf("\033[%sm", "35")
	case "cyan":
		return fmt.Sprintf("\033[%sm", "36")
	case "white":
		return fmt.Sprintf("\033[%sm", "37")

	default:
		log.Println("unknown color, defaulting to white")
		return fmt.Sprintf("\033[%sm", "37")
	}
}

// coloredLogger stores details about the logger.
type coloredLogger struct {
	color     string
	colorCode string
	prefix    string
}

// logger handles outputing colored logs. Use standard logging format just to be
// consistent with default golang logging and what users would most likely expect.
//
// The colorable package is needed to handle colorizing Windows logs; if we didn't
// care about Windows we could just use os.Stderr instead.
var logger = log.New(colorable.NewColorableStderr(), "", log.LstdFlags)

// newLogger returns a coloredLogger for calling Printf on with the resulting log
// colored and prefixed accordingly.
func newLogger(prefix, color string) coloredLogger {
	colorCode := getColorCode(color)
	return coloredLogger{color, colorCode, prefix}
}

// Printf calls log.Printf with color sequences surrounding some of the text.
func (c *coloredLogger) Printf(format string, v ...interface{}) {
	resetCode := fmt.Sprintf("\033[%sm", "0")

	format = fmt.Sprintf("%s%s |%s %s", c.colorCode, c.prefix, resetCode, format)
	logger.Printf(format, v...)
}