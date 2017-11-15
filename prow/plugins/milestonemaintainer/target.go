// /*
// Copyright 2017 The Kubernetes Authors.

// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at

//     http://www.apache.org/licenses/LICENSE-2.0

// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
// */

package milestonemaintainer

// // TODO replace notification

// var (
// 	// milestoneStateLabels is the set of milestone labels applied by
// 	// the plugin.  statusApprovedLabel is not included because it is
// 	// applied manually rather than by the plugin.
// 	milestoneStateLabels = []string{
// 		milestoneLabelsIncompleteLabel,
// 		milestoneNeedsApprovalLabel,
// 		milestoneNeedsAttentionLabel,
// 		milestoneRemovedLabel,
// 	}
// )

// type pluginComment struct {
// 	body      string
// 	createdAt time.Time

// 	// TODO remove the need for this - use the id instead
// 	source githubapi.IssueComment
// }

// type milestoneTarget interface {
// 	releaseMilestone() (string, bool)
// 	latestNotificationComment(botName string) (notification, bool)
// 	addLabel(label string) error
// 	hasLabel(label string) bool
// 	removeLabel(label) error
// 	clearMilestone() bool
// }

// type mungerMilestoneTarget struct {
// 	obj *github.MungeObject
// }

// func newMungerMilestoneTarget(obj *github.MungeObject) *mungerMilestoneTarget {
// 	return &mungerMilestoneTarget{
// 		obj: obj,
// 	}
// }

// // releaseMilestone returns the name of the 'release' milestone or an
// // empty string if none found.
// func (t *mungerMilestoneTarget) releaseMilestone() (string, bool) {
// 	return t.obj.ReleaseMilestone()
// }

// // latestNotificationComment returns the most recent notification
// // comment posted by the plugin.
// //
// // Since the plugin is careful to remove existing comments before
// // adding new ones, only a single notification comment should exist.
// func (t *mungerMilestoneTarget) latestNotificationComment(botName string) (*pluginComment, bool) {
// 	issueComments, ok := t.obj.ListComments()
// 	if !ok {
// 		return nil, false
// 	}
// 	comments := c.FromIssueComments(issueComments)
// 	notificationMatcher := c.MungerNotificationName(milestoneNotifierName, botName)
// 	notifications := c.FilterComments(comments, notificationMatcher)
// 	lastComment := notifications.GetLast()
// 	if lastComment == nil {
// 		return nil, true
// 	}
// 	return &pluginComment{
// 		body:      lastComment.Body,
// 		createdAt: lastComment.CreatedAt,
// 		source:    lastComment.Source.(*githubapi.IssueComment),
// 	}, true
// }

// func (t *mungerMilestoneTarget) addLabel(labelName string) error {
// 	return t.obj.AddLabel(labelName)
// }

// func (t *mungerMilestoneTarget) hasLabel(labelName string) bool {
// 	return t.obj.HasLabel(labelName)
// }

// func (t *mungerMilestoneTarget) removeLabel(labelName string) error {
// 	return t.obj.RemoveLabel(labelName)
// }

// // updateMilestoneStateLabel ensures that the given milestone state
// // label is the only state label set on the given issue.
// func (t *mungerMilestoneTarget) updateMilestoneStateLabel(labelName string) bool {
// 	if len(labelName) > 0 && !t.obj.HasLabel(labelName) {
// 		if err := obj.AddLabel(labelName); err != nil {
// 			return false
// 		}
// 	}
// 	for _, stateLabel := range milestoneStateLabels {
// 		if stateLabel != labelName && t.obj.HasLabel(stateLabel) {
// 			if err := obj.RemoveLabel(stateLabel); err != nil {
// 				return false
// 			}
// 		}
// 	}
// 	return true
// }

// // clearMilestone will remove a milestone if present
// func (t *mungerMilestoneTarget) clearMilestone() bool {
// 	return t.obj.clearMilestone()
// }

// func (t *mungerMilestoneTarget) setNotificationComment(botName string, change issueChange) bool {
// 	comment, ok := t.latestNotificationComment(botName)
// 	if !ok {
// 		return false
// 	}
// 	if !notificationIsCurrent(change.notification, comment, change.commentInterval) {
// 		if comment != nil {
// 			if err := t.obj.DeleteComment(comment.source); err != nil {
// 				return false
// 			}
// 		}
// 		if err := change.notification.Post(t.obj); err != nil {
// 			return false
// 		}
// 	}
// 	return true
// }

// func (t *mungerMilestoneTarget) mentions() string {
// 	return mungerutil.GetIssueUsers(t.obj.Issue).AllUsers().Mention().Join()
// }

// // notificationIsCurrent indicates whether the given notification
// // matches the most recent notification comment and the comment
// // interval - if provided - has not been exceeded.
// func notificationIsCurrent(notification *c.Notification, comment *c.Comment, commentInterval *time.Duration) bool {
// 	oldNotification := c.ParseNotification(comment)
// 	notificationsEqual := oldNotification != nil && oldNotification.Equal(notification)
// 	return notificationsEqual && (commentInterval == nil || comment != nil && comment.CreatedAt != nil && time.Since(*comment.CreatedAt) < *commentInterval)
// }

// // gracePeriodRemaining returns the difference between the start of
// // the grace period and the grace period interval. Returns nil the
// // grace period start cannot be determined.
// func gracePeriodRemaining(target *milestoneTarget, botName, labelName string, gracePeriod time.Duration, defaultStart time.Time, isBlocker bool) (*time.Duration, bool) {
// 	if isBlocker {
// 		return nil, true
// 	}
// 	tempStart := gracePeriodStart(target, botName, labelName, defaultStart)
// 	if tempStart == nil {
// 		return nil, false
// 	}
// 	start := *tempStart

// 	remaining := -time.Since(start.Add(gracePeriod))
// 	return &remaining, true
// }

// // gracePeriodStart determines when the grace period for the given
// // object should start as is indicated by when the
// // milestone-labels-incomplete label was last applied. If the label
// // is not set, the default will be returned. nil will be returned if
// // an error occurs while accessing the object's label events.
// func gracePeriodStart(target *milestoneTarget, botName, labelName string, defaultStart time.Time) *time.Time {
// 	if !target.HasLabel(labelName) {
// 		return &defaultStart
// 	}

// 	return labelLastCreatedAt(target, botName, labelName)
// }

// // labelLastCreatedAt returns the time at which the given label was
// // last applied to the given github object. Returns nil if an error
// // occurs during event retrieval or if the label has never been set.
// func labelLastCreatedAt(target *milestoneTarget, botName, labelName string) *time.Time {
// 	events, ok := target.GetEvents()
// 	if !ok {
// 		return nil
// 	}

// 	labelMatcher := event.And([]event.Matcher{
// 		event.AddLabel{},
// 		event.LabelName(labelName),
// 		event.Actor(botName),
// 	})
// 	labelEvents := event.FilterEvents(events, labelMatcher)
// 	lastAdded := labelEvents.GetLast()
// 	if lastAdded != nil {
// 		return lastAdded.CreatedAt
// 	}
// 	return nil
// }
