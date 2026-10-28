// Package logging provides the structured logger of the server.
//
// Two streams are kept: system logs describing the state of the server and
// user logs describing what the clients did. Both streams are written to
// rotating daily files and kept in a bounded in-memory ring so that the
// administration console can display them without touching the file system.
package logging

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Level is the severity of a log record.
type Level int

// Log levels ordered by severity.
const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
)

func (l Level) String() string {
	switch l {
	case LevelDebug:
		return "DEBUG"
	case LevelInfo:
		return "INFO"
	case LevelWarn:
		return "WARN"
	default:
		return "ERROR"
	}
}

// ParseLevel converts a level name into a Level.
func ParseLevel(name string) Level {
	switch strings.ToUpper(strings.TrimSpace(name)) {
	case "DEBUG":
		return LevelDebug
	case "WARN", "WARNING":
		return LevelWarn
	case "ERROR":
		return LevelError
	default:
		return LevelInfo
	}
}

// SystemRecord is a system log entry.
type SystemRecord struct {
	Time    string         `json:"time"`
	Level   string         `json:"level"`
	Message string         `json:"message"`
	Data    map[string]any `json:"data,omitempty"`
}

// UserRecord is a user action log entry.
type UserRecord struct {
	Time   string         `json:"time"`
	UserID string         `json:"userId"`
	Action string         `json:"action"`
	Detail map[string]any `json:"detail,omitempty"`
}

const ringSize = 2000

// Logger writes system and user logs.
type Logger struct {
	mu        sync.Mutex
	dir       string
	level     Level
	out       io.Writer
	systemLog []SystemRecord
	userLog   []UserRecord
}

// New creates a logger writing daily files into dir.
func New(dir string, level Level) (*Logger, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &Logger{dir: dir, level: level, out: os.Stdout}, nil
}

// SetOutput redirects the console output, mostly useful in tests.
func (l *Logger) SetOutput(w io.Writer) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.out = w
}

// System logs a server side event.
func (l *Logger) System(level Level, message string, data ...map[string]any) {
	if level < l.level {
		return
	}
	payload := firstPayload(data)
	record := SystemRecord{
		Time:    time.Now().Format("2006-01-02T15:04:05.000Z07:00"),
		Level:   level.String(),
		Message: message,
		Data:    payload,
	}
	line := formatLine(record.Time, record.Level, message, payload)

	l.mu.Lock()
	l.systemLog = append(l.systemLog, record)
	if len(l.systemLog) > ringSize {
		l.systemLog = l.systemLog[len(l.systemLog)-ringSize:]
	}
	out := l.out
	l.mu.Unlock()

	if out != nil {
		fmt.Fprintln(out, colorize(level, line))
	}
	l.appendFile("system", line)
}

// Debug logs at debug level.
func (l *Logger) Debug(message string, data ...map[string]any) {
	l.System(LevelDebug, message, data...)
}

// Info logs at info level.
func (l *Logger) Info(message string, data ...map[string]any) { l.System(LevelInfo, message, data...) }

// Warn logs at warning level.
func (l *Logger) Warn(message string, data ...map[string]any) { l.System(LevelWarn, message, data...) }

// Error logs at error level.
func (l *Logger) Error(message string, data ...map[string]any) {
	l.System(LevelError, message, data...)
}

// User logs a client action.
func (l *Logger) User(userID, action string, detail ...map[string]any) {
	payload := firstPayload(detail)
	record := UserRecord{
		Time:   time.Now().Format("2006-01-02T15:04:05.000Z07:00"),
		UserID: userID,
		Action: action,
		Detail: payload,
	}
	line := formatLine(record.Time, "USER", "["+userID+"] "+action, payload)

	l.mu.Lock()
	l.userLog = append(l.userLog, record)
	if len(l.userLog) > ringSize {
		l.userLog = l.userLog[len(l.userLog)-ringSize:]
	}
	out := l.out
	l.mu.Unlock()

	if out != nil {
		fmt.Fprintln(out, "\x1b[90m"+line+"\x1b[0m")
	}
	l.appendFile("user", line)
}

// SystemRecords returns the most recent system records, newest first.
func (l *Logger) SystemRecords(limit int, level string) []SystemRecord {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := []SystemRecord{}
	for i := len(l.systemLog) - 1; i >= 0 && len(out) < limit; i-- {
		if level != "" && !strings.EqualFold(l.systemLog[i].Level, level) {
			continue
		}
		out = append(out, l.systemLog[i])
	}
	return out
}

// UserRecords returns the most recent user records, newest first.
func (l *Logger) UserRecords(limit int, userID, action string) []UserRecord {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := []UserRecord{}
	for i := len(l.userLog) - 1; i >= 0 && len(out) < limit; i-- {
		if userID != "" && l.userLog[i].UserID != userID {
			continue
		}
		if action != "" && !strings.EqualFold(l.userLog[i].Action, action) {
			continue
		}
		out = append(out, l.userLog[i])
	}
	return out
}

// LogFiles returns the available daily log files per stream.
func (l *Logger) LogFiles() map[string][]string {
	out := map[string][]string{"system": {}, "user": {}}
	entries, err := os.ReadDir(l.dir)
	if err != nil {
		return out
	}
	for _, e := range entries {
		name := e.Name()
		switch {
		case strings.HasPrefix(name, "system-"):
			out["system"] = append(out["system"], name)
		case strings.HasPrefix(name, "user-"):
			out["user"] = append(out["user"], name)
		}
	}
	return out
}

// LogFileContent returns the tail of a daily log file.
func (l *Logger) LogFileContent(stream, date string, lines int) string {
	if stream != "system" && stream != "user" {
		return "日志类型必须是 system 或 user"
	}
	if date == "" {
		date = time.Now().Format("2006-01-02")
	}
	path := filepath.Join(l.dir, fmt.Sprintf("%s-%s.log", stream, date))
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Sprintf("日志文件不存在: %s-%s.log", stream, date)
	}
	all := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if lines <= 0 || lines > len(all) {
		lines = len(all)
	}
	return strings.Join(all[len(all)-lines:], "\n")
}

// Cleanup removes log files older than the given number of days.
func (l *Logger) Cleanup(days int) int {
	entries, err := os.ReadDir(l.dir)
	if err != nil {
		return 0
	}
	cutoff := time.Now().AddDate(0, 0, -days)
	removed := 0
	for _, e := range entries {
		date, ok := extractDate(e.Name())
		if !ok {
			continue
		}
		if date.Before(cutoff) {
			if removeErr := os.Remove(filepath.Join(l.dir, e.Name())); removeErr == nil {
				removed++
			}
		}
	}
	return removed
}

func (l *Logger) appendFile(stream, line string) {
	name := fmt.Sprintf("%s-%s.log", stream, time.Now().Format("2006-01-02"))
	path := filepath.Join(l.dir, name)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintln(f, line)
}

// firstPayload returns the first payload of a variadic argument list.
func firstPayload(data []map[string]any) map[string]any {
	if len(data) == 0 {
		return nil
	}
	return data[0]
}

func formatLine(timeStr, level, message string, data map[string]any) string {
	line := fmt.Sprintf("[%s] [%s] %s", timeStr, level, message)
	if len(data) > 0 {
		if encoded, err := json.Marshal(data); err == nil {
			line += " " + string(encoded)
		}
	}
	return line
}

func colorize(level Level, line string) string {
	switch level {
	case LevelDebug:
		return "\x1b[90m" + line + "\x1b[0m"
	case LevelInfo:
		return "\x1b[36m" + line + "\x1b[0m"
	case LevelWarn:
		return "\x1b[33m" + line + "\x1b[0m"
	default:
		return "\x1b[31m" + line + "\x1b[0m"
	}
}

func extractDate(name string) (time.Time, bool) {
	idx := strings.Index(name, "-")
	if idx < 0 {
		return time.Time{}, false
	}
	rest := name[idx+1:]
	if len(rest) < 10 {
		return time.Time{}, false
	}
	parsed, err := time.Parse("2006-01-02", rest[:10])
	if err != nil {
		return time.Time{}, false
	}
	return parsed, true
}
