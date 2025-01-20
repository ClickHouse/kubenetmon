package meta

import (
	"fmt"
	"log"
	"runtime"
	"strings"

	"github.com/rs/zerolog"
	"github.com/sirupsen/logrus"
)

type VersionInfo struct {
	Name      string
	Version   string
	GitCommit string
}

type BasicLogger interface {
	Info(msg string, keysAndValues ...any)
}

type SugaredLogger interface {
	Info(args ...any)
}

const (
	msgVersionInfo = "version info"
)

func PrintVersionStd(l *log.Logger, info VersionInfo) {
	var strs []string
	for k, v := range prepareLogFields(info) {
		strs = append(strs, fmt.Sprintf("%s=%s", k, v))
	}
	l.Printf("%s %s\n", msgVersionInfo, strings.Join(strs, " "))
}

func PrintVersionBasic(l BasicLogger, info VersionInfo) {
	kvs := prepareLogFields(info)
	args := make([]any, len(kvs)*2)
	for k, v := range kvs {
		args = append(args, k, v)
	}
	l.Info(msgVersionInfo, args...)
}

func PrintVersionZerolog(l *zerolog.Logger, info VersionInfo) {
	e := l.Info()
	for k, v := range prepareLogFields(info) {
		e.Str(k, v)
	}
	e.Msg(msgVersionInfo)
}

func PrintVersionLogrus(l *logrus.Logger, info VersionInfo) {
	fields := make(logrus.Fields)
	for k, v := range prepareLogFields(info) {
		fields[k] = v
	}
	l.WithFields(fields).Info(msgVersionInfo)
}

func PrintVersionSugared(l SugaredLogger, info VersionInfo) {
	kvs := prepareLogFields(info)
	args := make([]any, len(kvs)*2+1)
	args = append(args, msgVersionInfo)
	for k, v := range kvs {
		args = append(args, k, v)
	}
	l.Info(args...)
}

func prepareLogFields(info VersionInfo) map[string]string {
	sanitizeVersionInfo(&info)

	goVersion, _ := strings.CutPrefix(runtime.Version(), "go")

	return map[string]string{
		"component":  info.Name,
		"version":    info.Version,
		"git-commit": info.GitCommit,
		"go-version": goVersion,
		"go-os":      runtime.GOOS,
		"go-arch":    runtime.GOARCH,
	}
}

func sanitizeVersionInfo(info *VersionInfo) {
	if info.Name == "" {
		info.Name = "<unspecified>"
	}
	if info.Version == "" {
		info.Version = "snapshot"
	}
	if info.GitCommit == "" {
		info.GitCommit = "unspecified"
	}
}
