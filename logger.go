package main

import (
	"fmt"
	"io"
	"os"
	"strings"
)

var (
	logwriter io.Writer
)

func initLogging(stderr bool) {
	if stderr {
		logwriter = os.Stderr
	} else {
		logwriter = os.Stdout
	}
}

func logRaw(format string, args ...interface{}) {
	fmt.Fprintf(logwriter, format+"\n", args...)
}

func logTitle(format string, args ...interface{}) {
	logInfo(format, args...)

	title := strings.Repeat("-", len(fmt.Sprintf(format, args...)))
	if len(title) > 0 {
		logInfo(title)
	}
}

func logResultsInt(label string, value int) {
	if value > 0 {
		logResults(label, formatInt(value))
	}
}

func logResultsIntPostfix(label string, value int, postfix string) {
	if value > 0 {
		logInfo(fmt.Sprintf("%-15s %15s    %s", label, formatInt(value), postfix))
	}
}

func logResults(label, value string) {
	logInfo(fmt.Sprintf("%-15s %15s", label, value))
}

func logResultsPostfix(label, value, postfix string) {
	logInfo(fmt.Sprintf("%-15s %15s    %s", label, value, postfix))
}

func logInfo(format string, args ...interface{}) {
	if !StartParams.Quiet {
		logRaw(format, args...)
	}
}

func logWarn(format string, args ...interface{}) {
	format = "[WARN] " + format
	if !StartParams.Quiet {
		logRaw(format, args...)
	}
}

func logError(format string, args ...interface{}) {
	format = "[ERROR] " + format
	if !StartParams.Quiet {
		logRaw(format, args...)
	}
}

func logFatal(format string, args ...interface{}) {
	format = "\n[FATAL] " + format
	logRaw(format, args...)
	os.Exit(1)
}

func logFatalError(err error) {
	if err != nil {
		logFatal(err.Error())
	}
}
