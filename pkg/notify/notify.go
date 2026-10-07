package notify

import "os/exec"

type Notification struct {
	Title   string
	Message string
	OpenURL string
	Group   string
	Execute string
}

func Args(notification Notification) []string {
	args := []string{"-title", "digest", "-subtitle", notification.Title, "-message", notification.Message}
	if notification.OpenURL != "" {
		args = append(args, "-open", notification.OpenURL)
	}
	if notification.Group != "" {
		args = append(args, "-group", notification.Group)
	}
	if notification.Execute != "" {
		args = append(args, "-execute", notification.Execute)
	}
	return args
}

var Send = func(notification Notification) error {
	return exec.Command(senderBinary(), Args(notification)...).Run()
}
