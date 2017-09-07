/*
Copyright 2016 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package comment

import (
	"regexp"
	"strings"

	mgh "k8s.io/test-infra/mungegithub/github"
)

// Notification is a message sent by the bot. Easy to find and create.
type Notification struct {
	Name      string
	Arguments string
	Context   string
}

var (
	// Matches a notification: [NOTIFNAME] Arguments\n\nContext
	notificationRegex = regexp.MustCompile(`(?s)^\[([^\]\s]+)\] *?([^\n]*)(?:\n\n(.*))?`)
)

// ParseNotification attempts to read a notification from a comment
// Returns nil if the comment doesn't contain a notification
func ParseNotification(comment *Comment) *Notification {
	if comment == nil || comment.Body == nil {
		return nil
	}

	match := notificationRegex.FindStringSubmatch(*comment.Body)
	if match == nil {
		return nil
	}

	return &Notification{
		Name:      strings.ToUpper(match[1]),
		Arguments: strings.TrimSpace(match[2]),
		Context:   strings.TrimSpace(match[3]),
	}
}

// String converts the notification
func (n *Notification) String() string {
	str := "[" + strings.ToUpper(n.Name) + "]"

	args := strings.TrimSpace(n.Arguments)
	if args != "" {
		str += " " + args
	}

	context := strings.TrimSpace(n.Context)
	if context != "" {
		str += "\n\n" + context
	}

	return str
}

// Post a new notification on Github
func (n Notification) Post(obj *mgh.MungeObject) error {
	return obj.WriteComment(n.String())
}

// Equal compares this notification to the given notification for equivalence
func (n *Notification) Equal(o *Notification) bool {
	return (o != nil && n.Name == o.Name && n.Arguments == o.Arguments && n.Context == o.Context)
}
