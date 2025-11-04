package logging

import (
	"fmt"
	"log"
	"time"
)

type Component string

const (
	ComponentStartup  Component = "STARTUP"
	ComponentServer   Component = "SERVER"
	ComponentHashtab  Component = "HASHTAB"
	ComponentQMLDiff  Component = "QMLDIFF"
	ComponentHandler  Component = "HANDLER"
)

func Info(component Component, message string, args ...interface{}) {
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	msg := fmt.Sprintf(message, args...)
	log.Printf("[%s] [%s] %s", timestamp, component, msg)
}

func Error(component Component, message string, args ...interface{}) {
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	msg := fmt.Sprintf(message, args...)
	log.Printf("[%s] [%s] ERROR: %s", timestamp, component, msg)
}

func Warn(component Component, message string, args ...interface{}) {
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	msg := fmt.Sprintf(message, args...)
	log.Printf("[%s] [%s] WARN: %s", timestamp, component, msg)
}
