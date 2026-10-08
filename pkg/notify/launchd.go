package notify

import (
	"fmt"
	"html"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/AnudeepChPaul/digest/pkg/paths"
)

const LaunchAgentLabel = "com.digest.notify"

var launchctl = func(args ...string) error {
	output, err := exec.Command("launchctl", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("launchctl %v: %w: %s", args, err, output)
	}
	return nil
}

var LaunchAgentPath = func() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents", LaunchAgentLabel+".plist")
}

func LaunchAgentPlist(executable, logPath, pathEnv string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>%s</string>
	<key>ProgramArguments</key>
	<array>
		<string>%s</string>
		<string>notify-due</string>
	</array>
	<key>StartInterval</key>
	<integer>60</integer>
	<key>RunAtLoad</key>
	<true/>
	<key>EnvironmentVariables</key>
	<dict>
		<key>PATH</key>
		<string>%s</string>
	</dict>
	<key>StandardOutPath</key>
	<string>%s</string>
	<key>StandardErrorPath</key>
	<string>%s</string>
</dict>
</plist>
`, LaunchAgentLabel, html.EscapeString(executable), html.EscapeString(pathEnv), html.EscapeString(logPath), html.EscapeString(logPath))
}

func launchDomain() string {
	return fmt.Sprintf("gui/%d", os.Getuid())
}

func Install(executable, logPath string) error {
	sourceApp, err := terminalNotifierApp()
	if err != nil {
		return fmt.Errorf("install terminal-notifier first: %w", err)
	}
	if err := BuildNotifierApp(sourceApp, NotifierAppPath()); err != nil {
		return fmt.Errorf("build the digest notifier: %w", err)
	}
	plistPath := LaunchAgentPath()
	if err := os.MkdirAll(filepath.Dir(plistPath), 0755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(logPath), paths.PrivateDirMode); err != nil {
		return err
	}
	if err := createPrivateLog(logPath); err != nil {
		return err
	}
	if err := os.WriteFile(plistPath, []byte(LaunchAgentPlist(executable, logPath, os.Getenv("PATH"))), paths.PrivateFileMode); err != nil {
		return err
	}
	if err := os.Chmod(plistPath, paths.PrivateFileMode); err != nil {
		return err
	}
	_ = launchctl("bootout", launchDomain()+"/"+LaunchAgentLabel)
	return launchctl("bootstrap", launchDomain(), plistPath)
}

func createPrivateLog(logPath string) error {
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, paths.PrivateFileMode)
	if err != nil {
		return err
	}
	if err := logFile.Close(); err != nil {
		return err
	}
	return os.Chmod(logPath, paths.PrivateFileMode)
}

func Uninstall() error {
	_ = launchctl("bootout", launchDomain()+"/"+LaunchAgentLabel)
	if err := os.Remove(LaunchAgentPath()); err != nil && !os.IsNotExist(err) {
		return err
	}
	return os.RemoveAll(NotifierAppPath())
}

func Installed() bool {
	_, err := os.Stat(LaunchAgentPath())
	return err == nil
}
