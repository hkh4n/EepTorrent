package main

/*
A cross-platform I2P-only BitTorrent client.
Copyright (C) 2024 Haris Khan

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU General Public License for more details.

You should have received a copy of the GNU General Public License
along with this program.  If not, see <https://www.gnu.org/licenses/>.
*/

import (
	"bytes"
	"eeptorrent/lib/gui"
	"eeptorrent/lib/i2p"
	"github.com/sirupsen/logrus"
	"io"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"os"
)

var (
	log          = logrus.StandardLogger()
	maxRetries   = 100
	initialDelay = 2 * time.Second
	logFile      *os.File
	logFileMux   sync.Mutex
	logBuffer    bytes.Buffer
)

func init() {
	// Configure logrus
	log.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
		DisableColors: false,
	})
	log.SetOutput(io.MultiWriter(os.Stderr, &logBuffer))
	log.SetLevel(logrus.DebugLevel)
}

func main() {
	// Set up panic recovery
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("Recovered from panic: %v", r)
			i2p.Cleanup()
			os.Exit(1)
		}
	}()
	// Set up signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		log.Info("Received interrupt signal")
		i2p.Cleanup()
		os.Exit(0)
	}()
	gui.RunApp()
}
