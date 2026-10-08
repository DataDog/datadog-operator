// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package dashboard

import (
	"fmt"
	"io"
	"log"
	"log/slog"
	"os"
	"sync"

	"github.com/go-logr/logr"
	"github.com/go-logr/logr/funcr"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
)

// SilenceLogging redirects the process-wide loggers of the libraries the
// dashboard uses to logFile, or discards them when logFile is empty, so
// that nothing is written over the live view: klog (client-go),
// the standard log and slog packages (Helm), the controller-runtime logger
// and the client-go default warning handler. Call it before building any
// client. The returned function restores the previous stdlib log output
// and slog default, resets klog and the warning handler to their defaults
// (klog does not expose its previous settings) and closes the file; the
// controller-runtime logger, which can only be set once, then discards its
// output.
func SilenceLogging(logFile string) (restore func(), err error) {
	if logFile == "" {
		return silenceTo(io.Discard, nil), nil
	}
	f, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, fmt.Errorf("cannot open the log file: %w", err)
	}
	return silenceTo(&lockedWriter{w: f}, f), nil
}

// silenceTo redirects every logger to w and returns the restore function,
// which also closes c when not nil.
func silenceTo(w io.Writer, c io.Closer) func() {
	logger := logr.Discard()
	if w != io.Discard {
		logger = funcr.New(func(prefix, args string) { printLog(w, prefix, args) }, funcr.Options{})
	}

	prevLogOut, prevSlog := log.Writer(), slog.Default()
	log.SetOutput(w)
	slog.SetDefault(slog.New(slog.NewTextHandler(w, nil)))

	klog.SetLogger(logger)
	klog.SetOutput(w)
	klog.LogToStderr(false)

	ctrlOutput.set(w)
	ctrlLoggerOnce.Do(func() { ctrllog.SetLogger(funcr.New(ctrlOutput.print, funcr.Options{})) })
	rest.SetDefaultWarningHandler(rest.NewWarningWriter(w, rest.WarningWriterOptions{Deduplicate: true}))

	return func() {
		ctrlOutput.set(io.Discard)
		rest.SetDefaultWarningHandler(rest.WarningLogger{})
		klog.ClearLogger()
		klog.SetOutput(os.Stderr)
		klog.LogToStderr(true)
		slog.SetDefault(prevSlog)
		log.SetOutput(prevLogOut)
		if c != nil {
			_ = c.Close()
		}
	}
}

// lockedWriter serializes writes from the loggers sharing a file.
type lockedWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (l *lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(p)
}

func printLog(w io.Writer, prefix, args string) {
	if prefix != "" {
		fmt.Fprintln(w, prefix, args)
		return
	}
	fmt.Fprintln(w, args)
}

var (
	// ctrlLoggerOnce sets the controller-runtime logger, which can only be
	// set once per process, to write to ctrlOutput.
	ctrlLoggerOnce sync.Once
	ctrlOutput     = &switchWriter{}
)

// switchWriter is a writer whose destination can change.
type switchWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (s *switchWriter) set(w io.Writer) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.w = w
}

func (s *switchWriter) print(prefix, args string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.w != nil && s.w != io.Discard {
		printLog(s.w, prefix, args)
	}
}
