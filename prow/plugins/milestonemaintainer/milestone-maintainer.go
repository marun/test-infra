/*
Copyright 2017 The Kubernetes Authors.

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

package milestonemaintainer

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/plugins"

	"k8s.io/kubernetes/pkg/util/sets"
	"k8s.io/test-infra/mungegithub/mungers/approvers"
)

type milestoneState int

// milestoneStateConfig defines the label and notification
// configuration for a given milestone state.
type milestoneStateConfig struct {
	// The milestone label to apply to the label (all other milestone state labels will be removed)
	label string
	// The title of the notification message
	title string
	// Whether the notification should be repeated on the configured interval
	warnOnInterval bool
	// Whether sigs should be mentioned in the notification message
	notifySIGs bool
}

const (
	pluginName = "milestonemaintainer"

	milestoneNotifierName = "MilestoneNotifier"

	milestoneModeDev    = "dev"
	milestoneModeSlush  = "slush"
	milestoneModeFreeze = "freeze"

	milestoneCurrent        milestoneState = iota // No change is required.
	milestoneNeedsLabeling                        // One or more priority/*, kind/* and sig/* labels are missing.
	milestoneNeedsApproval                        // The status/needs-approval label is missing.
	milestoneNeedsAttention                       // A status/* label is missing or an update is required.
	milestoneNeedsRemoval                         // The issue needs to be removed from the milestone.

	milestoneLabelsIncompleteLabel = "milestone/incomplete-labels"
	milestoneNeedsApprovalLabel    = "milestone/needs-approval"
	milestoneNeedsAttentionLabel   = "milestone/needs-attention"
	milestoneRemovedLabel          = "milestone/removed"

	statusApprovedLabel   = "status/approved-for-milestone"
	statusInProgressLabel = "status/in-progress"

	blockerLabel = "priority/critical-urgent"

	sigLabelPrefix     = "sig/"
	sigMentionTemplate = "@kubernetes/sig-%s-misc"

	milestoneDetail = `<details>
<summary>Help</summary>
<ul>
 <li><a href="https://github.com/kubernetes/community/blob/master/contributors/devel/release/issues.md">Additional instructions</a></li>
 <li><a href="https://github.com/kubernetes/test-infra/blob/master/commands.md">Commands for setting labels</a></li>
</ul>
</details>
`

	milestoneMessageTemplate = `
{{- if .warnUnapproved}}
**Action required**: This {{.objType}} must have the {{.approvedLabel}} label applied by a SIG maintainer.{{.unapprovedRemovalWarning}}
{{end -}}
{{- if .removeUnapproved}}
**Important**: This {{.objType}} was missing the {{.approvedLabel}} label for more than {{.approvalGracePeriod}}.
{{end -}}
{{- if .warnMissingInProgress}}
**Action required**: During code {{.mode}}, {{.objTypePlural}} in the milestone should be in progress.
If this {{.objType}} is not being actively worked on, please remove it from the milestone.
If it is being worked on, please add the {{.inProgressLabel}} label so it can be tracked with other in-flight {{.objTypePlural}}.
{{end -}}
{{- if .warnUpdateRequired}}
**Action Required**: This {{.objType}} has not been updated since {{.lastUpdated}}. Please provide an update.
{{end -}}
{{- if .warnUpdateInterval}}
**Note**: This {{.objType}} is marked as {{.blockerLabel}}, and must be updated every {{.updateInterval}} during code {{.mode}}.

Example update:

` + "```" + `
ACK.  In progress
ETA: DD/MM/YYYY
Risks: Complicated fix required
` + "```" + `
{{end -}}
{{- if .warnNonBlockerRemoval}}
**Note**: If this {{.objType}} is not resolved or labeled as {{.blockerLabel}} by {{.freezeDate}} it will be moved out of the {{.milestone}}.
{{end -}}
{{- if .removeNonBlocker}}
**Important**: Code freeze is in effect and only {{.objTypePlural}} with {{.blockerLabel}} may remain in the {{.milestone}}.
{{end -}}
{{- if .warnIncompleteLabels}}
**Action required**: This {{.objType}} requires label changes.{{.incompleteLabelsRemovalWarning}}

{{range $index, $labelError := .labelErrors -}}
{{$labelError}}
{{end -}}
{{end -}}
{{- if .removeIncompleteLabels}}
**Important**: This {{.objType}} was missing labels required for the {{.milestone}} for more than {{.labelGracePeriod}}:

{{range $index, $labelError := .labelErrors -}}
{{$labelError}}
{{end}}
{{end -}}
{{- if .summarizeLabels -}}
<details{{if .onlySummary}} open{{end}}>
<summary>{{.objTypeTitle}} Labels</summary>

- {{range $index, $sigLabel := .sigLabels}}{{if $index}} {{end}}{{$sigLabel}}{{end}}: {{.objTypeTitle}} will be escalated to these SIGs if needed.
- {{.priorityLabel}}: {{.priorityDescription}}
- {{.kindLabel}}: {{.kindDescription}}
</details>
{{- end -}}
`
)

var (
	milestoneModes = sets.NewString(milestoneModeDev, milestoneModeSlush, milestoneModeFreeze)

	milestoneStateConfigs = map[milestoneState]milestoneStateConfig{
		milestoneCurrent: {
			title: "Milestone %s **Current**",
		},
		milestoneNeedsLabeling: {
			title:          "Milestone %s Labels **Incomplete**",
			label:          milestoneLabelsIncompleteLabel,
			warnOnInterval: true,
		},
		milestoneNeedsApproval: {
			title:          "Milestone %s **Needs Approval**",
			label:          milestoneNeedsApprovalLabel,
			warnOnInterval: true,
			notifySIGs:     true,
		},
		milestoneNeedsAttention: {
			title:          "Milestone %s **Needs Attention**",
			label:          milestoneNeedsAttentionLabel,
			warnOnInterval: true,
			notifySIGs:     true,
		},
		milestoneNeedsRemoval: {
			title:      "Milestone **Removed** From %s",
			label:      milestoneRemovedLabel,
			notifySIGs: true,
		},
	}

	// milestoneStateLabels is the set of milestone labels applied by
	// the plugin.  statusApprovedLabel is not included because it is
	// applied manually rather than by the plugin.
	milestoneStateLabels = []string{
		milestoneLabelsIncompleteLabel,
		milestoneNeedsApprovalLabel,
		milestoneNeedsAttentionLabel,
		milestoneRemovedLabel,
	}

	kindMap = map[string]string{
		"kind/bug":     "Fixes a bug discovered during the current release.",
		"kind/feature": "New functionality.",
		"kind/cleanup": "Adding tests, refactoring, fixing old bugs.",
	}

	priorityMap = map[string]string{
		blockerLabel:                  "Never automatically move %s out of a release milestone; continually escalate to contributor and SIG through all available channels.",
		"priority/important-soon":     "Escalate to the %s owners and SIG owner; move out of milestone after several unsuccessful escalation attempts.",
		"priority/important-longterm": "Escalate to the %s owners; move out of the milestone after 1 attempt.",
	}
)

type githubClient interface {
	AddLabel(owner, repo string, number int, label string) error
	BotName() (string, error)
	ClearMilestone(org, repo string, num int) error
	CreateComment(org, repo string, number int, comment string) error
	DeleteComment(org, repo string, ID int) error
	EditComment(org, repo string, ID int, comment string) error
	ListIssueComments(org, repo string, number int) ([]github.IssueComment, error)
	ListIssueEvents(org, repo string, num int) ([]github.ListedIssueEvent, error)
	RemoveLabel(org, repo string, number int, label string) error
}

// issueChange encapsulates changes to make to an issue.
type issueChange struct {
	// TODO replace notification?
	notification        *Notification
	label               string
	commentInterval     *time.Duration
	removeFromMilestone bool
}

type milestoneMaintainer struct {
	plugins.MilestoneMaintainer
	gc        githubClient
	log       *logrus.Entry
	milestone string
	mode      string
}

// Issue events to care about during dev
//   - labeled / unlabeled
//   - milestoned / demilestoned
//   - opened / reopened
// Issue events to care about during slush / freeze
//   - all?

func HandleIssue(gc githubClient, log *logrus.Entry, config plugins.MilestoneMaintainer, e github.IssueEvent) error {
	// Ignore closed issues
	if e.Issue.State == "closed" {
		log.Debug("Ignoring closed issue")
		return nil
	}

	// Ignore issues without a release milestone
	milestone := e.Issue.Milestone.ReleaseMilestone()
	if len(milestone) == 0 {
		log.Debug("Ignoring issue without a release milestone")
		return nil
	}

	// Validate configuration
	if err := config.Validate(); err != nil {
		return fmt.Errorf("Error validating config: %v", err)
	}

	// Ignore issues for milestones that aren't targeted
	mode, ok := config.Modes[milestone]
	if !ok {
		log.Debug("Ignoring issue that is not in a targeted milestone")
		return nil
	}

	log.Debug("Maintaining issue")

	m := &milestoneMaintainer{
		MilestoneMaintainer: config,
		gc:                  gc,
		log:                 log,
		milestone:           milestone,
		mode:                mode,
	}
	return m.maintainIssue(e)
}

// milestoneMode determines the release milestone and mode for the
// provided issue.  If a milestone is set and one of those targeted by
// the plugin, the milestone and mode will be returned along with a
// boolean indication of success.  Otherwise, if the milestone is not
// set or not targeted, a boolean indication of failure will be
// returned.
// func milestoneMode(config plugins.MilestoneMaintainer, issue github.Issue) (milestone string, mode string, success bool) {
// 	// Ignore issues that lack a released milestone
// 	milestone := issue.ReleaseMilestone()
// 	if len(milestone) == 0 {
// 		return "", "", false
// 	}

// 	// Ignore issues that aren't in a targeted milestone
// 	mode, exists := config.Modes[milestone]
// 	if !exists {
// 		return "", "", false
// 	}
// 	return milestone, mode, true
// }

// maintainIssue is the workhorse the will actually make updates to the issue
func (m *milestoneMaintainer) maintainIssue(e github.IssueEvent) error {
	change, err := m.issueChange(e)
	if err != nil {
		return err
	}
	if change == nil {
		return nil
	}

	if err := updateMilestoneStateLabel(m.gc, e, change.label); err != nil {
		return err
	}

	comment, notification, err := notificationComment(m.gc, e, m.log)
	if err != nil {
		return err
	}
	if comment == nil {
		return nil
	}

	if !notificationIsCurrent(change.notification, notification, comment, change.commentInterval) {
		if comment != nil {
			if err := m.gc.DeleteComment(e.Repo.Owner.Name, e.Repo.Name, comment.ID); err != nil {
				return err
			}
		}
		if err := m.gc.CreateComment(e.Repo.Owner.Name, e.Repo.Name, e.Issue.Number, change.notification.String()); err != nil {
			return err
		}
	}

	if change.removeFromMilestone {
		if err := m.gc.ClearMilestone(e.Repo.Owner.Name, e.Repo.Name, e.Issue.Number); err != nil {
			return err
		}
	}

	return nil
}

// issueChange computes the changes required to modify the state of
// the issue to reflect the milestone process. If a nil return value
// is returned, no action should be taken.
func (m *milestoneMaintainer) issueChange(e github.IssueEvent) (*issueChange, error) {
	icc, err := m.issueChangeConfig(e)
	if err != nil {
		return nil, err
	}
	if icc == nil {
		return nil, nil
	}

	messageBody := icc.messageBody()
	if messageBody == nil {
		return nil, nil
	}

	stateConfig := milestoneStateConfigs[icc.state]

	// TODO
	mentions := ""
	// mentions := target.mentions()
	// if stateConfig.notifySIGs {
	// 	sigMentions := icc.sigMentions()
	// 	if len(sigMentions) > 0 {
	// 		mentions = fmt.Sprintf("%s %s", mentions, sigMentions)
	// 	}
	// }

	message := fmt.Sprintf("%s\n\n%s\n%s", mentions, *messageBody, milestoneDetail)

	var commentInterval *time.Duration
	if stateConfig.warnOnInterval {
		commentInterval = &m.WarningInterval
	}

	// Ensure the title refers to the correct type (issue or pr)
	title := fmt.Sprintf(stateConfig.title, strings.Title(objTypeString(e.Issue)))

	return &issueChange{
		notification:        NewNotification(milestoneNotifierName, title, message),
		label:               stateConfig.label,
		removeFromMilestone: icc.state == milestoneNeedsRemoval,
		commentInterval:     commentInterval,
	}, nil
}

// issueChangeConfig computes the configuration required to determine
// the changes to make to an issue so that it reflects the milestone
// process. If a nil return value is returned, no action should be
// taken.
func (m *milestoneMaintainer) issueChangeConfig(e github.IssueEvent) (*issueChangeConfig, error) {
	updateInterval := m.updateInterval()

	// TODO objTypeString(obj)
	objType := "issue"

	icc := &issueChangeConfig{
		enabledSections: sets.String{},
		templateArguments: map[string]interface{}{
			"approvalGracePeriod": durationToMaxDays(m.ApprovalGracePeriod),
			"approvedLabel":       quoteLabel(statusApprovedLabel),
			"blockerLabel":        quoteLabel(blockerLabel),
			"freezeDate":          m.FreezeDate,
			"inProgressLabel":     quoteLabel(statusInProgressLabel),
			"labelGracePeriod":    durationToMaxDays(m.LabelGracePeriod),
			"milestone":           fmt.Sprintf("%s milestone", m.milestone),
			"mode":                m.mode,
			"objType":             objType,
			"objTypePlural":       fmt.Sprintf("%ss", objType),
			"objTypeTitle":        strings.Title(objType),
			"updateInterval":      durationToMaxDays(updateInterval),
		},
		sigLabels: []string{},
	}

	issue := e.Issue
	isBlocker := issue.HasLabel(blockerLabel)

	if kind, priority, sigs, labelErrors := checkLabels(issue.Labels); len(labelErrors) == 0 {
		icc.summarizeLabels(objType, kind, priority, sigs)
		if !issue.HasLabel(statusApprovedLabel) {
			if isBlocker {
				icc.warnUnapproved(nil, objType, m.milestone)
			} else {
				removeAfter, err := gracePeriodRemaining(m.gc, e, milestoneNeedsApprovalLabel, m.ApprovalGracePeriod, time.Now(), false)
				if err != nil {
					return nil, err
				}

				if removeAfter == nil || *removeAfter >= 0 {
					icc.warnUnapproved(removeAfter, objType, m.milestone)
				} else {
					icc.removeUnapproved()
				}
			}
			return icc, nil
		}

		if m.mode == milestoneModeDev {
			// Status and updates are not required for dev mode
			return icc, nil
		}

		if m.mode == milestoneModeFreeze && !isBlocker {
			icc.removeNonBlocker()
			return icc, nil
		}

		if !issue.HasLabel(statusInProgressLabel) {
			icc.warnMissingInProgress()
		}

		// TODO
		// if !isBlocker {
		// 	icc.enableSection("warnNonBlockerRemoval")
		// } else if updateInterval > 0 {
		// 	lastUpdateTime, ok := findLastModificationTime(obj)
		// 	if !ok {
		// 		return nil
		// 	}

		// 	durationSinceUpdate := time.Since(*lastUpdateTime)
		// 	if durationSinceUpdate > updateInterval {
		// 		icc.warnUpdateRequired(*lastUpdateTime)
		// 	}
		// 	icc.enableSection("warnUpdateInterval")
		// }
	} else {
		removeAfter, err := gracePeriodRemaining(m.gc, e, milestoneLabelsIncompleteLabel, m.LabelGracePeriod, time.Now(), isBlocker)
		if err != nil {
			return nil, err
		}

		if removeAfter == nil || *removeAfter >= 0 {
			icc.warnIncompleteLabels(removeAfter, labelErrors, objType, m.milestone)
		} else {
			icc.removeIncompleteLabels(labelErrors)
		}
	}
	return icc, nil
}

func (m *milestoneMaintainer) updateInterval() time.Duration {
	if m.mode == milestoneModeSlush {
		return m.SlushUpdateInterval
	}
	if m.mode == milestoneModeFreeze {
		return m.FreezeUpdateInterval
	}
	return 0
}

func objTypeString(issue github.Issue) string {
	if issue.IsPullRequest() {
		return "pull request"
	}
	return "issue"
}

// issueChangeConfig is the config required to change an issue (via
// comments and labeling) to reflect the reuqirements of the milestone
// maintainer.
type issueChangeConfig struct {
	state             milestoneState
	enabledSections   sets.String
	sigLabels         []string
	templateArguments map[string]interface{}
}

func (icc *issueChangeConfig) messageBody() *string {
	for _, sectionName := range icc.enabledSections.List() {
		// If an issue will be removed from the milestone, suppress non-removal sections
		if icc.state != milestoneNeedsRemoval || strings.HasPrefix(sectionName, "remove") {
			icc.templateArguments[sectionName] = true
		}
	}

	icc.templateArguments["onlySummary"] = icc.state == milestoneCurrent

	// TODO switch to using helper from approve/approvers/owners.go
	return approvers.GenerateTemplateOrFail(milestoneMessageTemplate, "message", icc.templateArguments)
}

func (icc *issueChangeConfig) enableSection(sectionName string) {
	icc.enabledSections.Insert(sectionName)
}

func (icc *issueChangeConfig) summarizeLabels(objType, kindLabel, priorityLabel string, sigLabels []string) {
	icc.enableSection("summarizeLabels")
	icc.state = milestoneCurrent
	icc.sigLabels = sigLabels
	quotedSigLabels := []string{}
	for _, sigLabel := range sigLabels {
		quotedSigLabels = append(quotedSigLabels, quoteLabel(sigLabel))
	}
	arguments := map[string]interface{}{
		"kindLabel":           quoteLabel(kindLabel),
		"kindDescription":     kindMap[kindLabel],
		"priorityLabel":       quoteLabel(priorityLabel),
		"priorityDescription": fmt.Sprintf(priorityMap[priorityLabel], objType),
		"sigLabels":           quotedSigLabels,
	}
	for k, v := range arguments {
		icc.templateArguments[k] = v
	}
}

func (icc *issueChangeConfig) warnUnapproved(removeAfter *time.Duration, objType, milestone string) {
	icc.enableSection("warnUnapproved")
	icc.state = milestoneNeedsApproval
	var warning string
	if removeAfter != nil {
		warning = fmt.Sprintf(" If the label is not applied within %s, the %s will be moved out of the %s milestone.",
			durationToMaxDays(*removeAfter), objType, milestone)
	}
	icc.templateArguments["unapprovedRemovalWarning"] = warning

}

func (icc *issueChangeConfig) removeUnapproved() {
	icc.enableSection("removeUnapproved")
	icc.state = milestoneNeedsRemoval
}

func (icc *issueChangeConfig) removeNonBlocker() {
	icc.enableSection("removeNonBlocker")
	icc.state = milestoneNeedsRemoval
}

func (icc *issueChangeConfig) warnMissingInProgress() {
	icc.enableSection("warnMissingInProgress")
	icc.state = milestoneNeedsAttention
}

func (icc *issueChangeConfig) warnUpdateRequired(lastUpdated time.Time) {
	icc.enableSection("warnUpdateRequired")
	icc.state = milestoneNeedsAttention
	icc.templateArguments["lastUpdated"] = lastUpdated.Format("Jan 2")
}

func (icc *issueChangeConfig) warnIncompleteLabels(removeAfter *time.Duration, labelErrors []string, objType, milestone string) {
	icc.enableSection("warnIncompleteLabels")
	icc.state = milestoneNeedsLabeling
	var warning string
	if removeAfter != nil {
		warning = fmt.Sprintf(" If the required changes are not made within %s, the %s will be moved out of the %s milestone.",
			durationToMaxDays(*removeAfter), objType, milestone)
	}
	icc.templateArguments["incompleteLabelsRemovalWarning"] = warning
	icc.templateArguments["labelErrors"] = labelErrors
}

func (icc *issueChangeConfig) removeIncompleteLabels(labelErrors []string) {
	icc.enableSection("removeIncompleteLabels")
	icc.state = milestoneNeedsRemoval
	icc.templateArguments["labelErrors"] = labelErrors
}

func (icc *issueChangeConfig) sigMentions() string {
	mentions := []string{}
	for _, label := range icc.sigLabels {
		sig := strings.TrimPrefix(label, sigLabelPrefix)
		target := fmt.Sprintf(sigMentionTemplate, sig)
		mentions = append(mentions, target)
	}
	return strings.Join(mentions, " ")
}

// notificationComment returns the comment (and the notification
// parsed from it) posted to the issue by the plugin.
//
// Since the plugin is careful to remove existing comments before
// adding new ones, only a single notification comment should exist.
func notificationComment(gc githubClient, e github.IssueEvent, log *logrus.Entry) (*github.IssueComment, *Notification, error) {
	comments, err := gc.ListIssueComments(e.Repo.Owner.Name, e.Repo.Name, e.Issue.Number)
	if err != nil {
		return nil, nil, err
	}

	botName, err := gc.BotName()
	if err != nil {
		return nil, nil, err
	}

	for _, comment := range comments {
		if comment.User.Login != botName {
			continue
		}
		notif := ParseNotification(comment.Body)
		if notif == nil {
			continue
		}

		// Only one comment will ever exist for the notifier
		if strings.ToUpper(notif.Name) == strings.ToUpper(milestoneNotifierName) {
			return &comment, notif, nil
		}
	}
	return nil, nil, nil
}

// notificationIsCurrent indicates whether the given notification
// matches the most recent notification comment and the comment
// interval - if provided - has not been exceeded.
func notificationIsCurrent(oldNotification, newNotification *Notification, oldComment *github.IssueComment, commentInterval *time.Duration) bool {
	notificationsEqual := oldNotification != nil && oldNotification.Equal(newNotification)
	return notificationsEqual && (commentInterval == nil || oldComment != nil && time.Since(oldComment.CreatedAt) < *commentInterval)
}

// gracePeriodRemaining returns the difference between the start of
// the grace period and the grace period interval. Returns nil the
// grace period start cannot be determined.
func gracePeriodRemaining(gc githubClient, e github.IssueEvent, labelName string, gracePeriod time.Duration, defaultStart time.Time, isBlocker bool) (*time.Duration, error) {
	if isBlocker {
		return nil, nil
	}

	tempStart, err := gracePeriodStart(gc, e, labelName, defaultStart)
	if err != nil {
		return nil, err
	}
	if tempStart == nil {
		return nil, nil
	}
	start := *tempStart

	remaining := -time.Since(start.Add(gracePeriod))
	return &remaining, nil
}

// gracePeriodStart determines when the grace period for the given
// object should start as is indicated by when the given label was
// last applied. If the label is not set, the default will be
// returned. nil will be returned if an error occurs while accessing
// the issue's events.
func gracePeriodStart(gc githubClient, e github.IssueEvent, labelName string, defaultStart time.Time) (*time.Time, error) {
	if !e.Issue.HasLabel(labelName) {
		return &defaultStart, nil
	}

	return labelLastCreatedAt(gc, e, labelName)
}

// labelLastCreatedAt returns the time at which the given label was
// last applied to the given issue. Returns nil if an error occurs
// during event retrieval or if the label has never been set.
func labelLastCreatedAt(gc githubClient, e github.IssueEvent, labelName string) (*time.Time, error) {
	events, err := gc.ListIssueEvents(e.Repo.Owner.Name, e.Repo.Name, e.Issue.Number)
	if err != nil {
		return nil, err
	}

	botName, err := gc.BotName()
	if err != nil {
		return nil, err
	}

	// Find all instances of the label being applied by the bot
	matchedEvents := []github.ListedIssueEvent{}
	for _, event := range events {
		if event.Event == github.IssueActionLabeled &&
			event.Actor.Login == botName &&
			event.Label.Name == labelName {
			matchedEvents = append(matchedEvents, event)
		}
	}

	// Return the creation timestamp of the most recent application
	if len(matchedEvents) > 0 {
		return &matchedEvents[len(matchedEvents)-1].CreatedAt, nil
	}
	return nil, nil
}

// checkLabels validates that the given labels are consistent with the
// requirements for an issue remaining in its chosen milestone.
// Returns the values of required labels (if present) and a slice of
// errors (where labels are not correct).
func checkLabels(labels []github.Label) (kindLabel, priorityLabel string, sigLabels []string, labelErrors []string) {
	labelErrors = []string{}
	var err error

	kindLabel, err = uniqueLabelName(labels, kindMap)
	if err != nil || len(kindLabel) == 0 {
		kindLabels := formatLabelString(kindMap)
		labelErrors = append(labelErrors, fmt.Sprintf("_**kind**_: Must specify exactly one of %s.", kindLabels))
	}

	priorityLabel, err = uniqueLabelName(labels, priorityMap)
	if err != nil || len(priorityLabel) == 0 {
		priorityLabels := formatLabelString(priorityMap)
		labelErrors = append(labelErrors, fmt.Sprintf("_**priority**_: Must specify exactly one of %s.", priorityLabels))
	}

	sigLabels = sigLabelNames(labels)
	if len(sigLabels) == 0 {
		labelErrors = append(labelErrors, fmt.Sprintf("_**sig owner**_: Must specify at least one label prefixed with `%s`.", sigLabelPrefix))
	}

	return
}

// uniqueLabelName determines which label of a set indicated by a map
// - if any - is present in the given slice of labels. Returns an
// error if the slice contains more than one label from the set.
func uniqueLabelName(labels []github.Label, labelMap map[string]string) (string, error) {
	var labelName string
	for _, label := range labels {
		_, exists := labelMap[label.Name]
		if exists {
			if len(labelName) == 0 {
				labelName = label.Name
			} else {
				return "", errors.New("Found more than one matching label")
			}
		}
	}
	return labelName, nil
}

// sigLabelNames returns a slice of the 'sig/' prefixed labels set on the issue.
func sigLabelNames(labels []github.Label) []string {
	labelNames := []string{}
	for _, label := range labels {
		if strings.HasPrefix(label.Name, sigLabelPrefix) {
			labelNames = append(labelNames, label.Name)
		}
	}
	return labelNames
}

// formatLabelString converts a map to a string in the format "`key-foo`, `key-bar`".
func formatLabelString(labelMap map[string]string) string {
	labelList := []string{}
	for k := range labelMap {
		labelList = append(labelList, quoteLabel(k))
	}
	sort.Strings(labelList)

	maxIndex := len(labelList) - 1
	if maxIndex == 0 {
		return labelList[0]
	}
	return strings.Join(labelList[0:maxIndex], ", ") + " or " + labelList[maxIndex]
}

// quoteLabel formats a label name as inline code in markdown (e.g. `labelName`)
func quoteLabel(label string) string {
	if len(label) > 0 {
		return fmt.Sprintf("`%s`", label)
	}
	return label
}

// updateMilestoneStateLabel ensures that the given milestone state
// label is the only state label set on the given issue.
func updateMilestoneStateLabel(gc githubClient, e github.IssueEvent, labelName string) error {
	org := e.Repo.Owner.Name
	repo := e.Repo.Name
	num := e.Issue.Number
	if len(labelName) > 0 && !e.Issue.HasLabel(labelName) {
		if err := gc.AddLabel(org, repo, num, labelName); err != nil {
			return fmt.Errorf("error adding label %s to %s/%s #%d: %v", labelName, org, repo, num, err)
		}
	}
	for _, stateLabel := range milestoneStateLabels {
		if stateLabel != labelName && e.Issue.HasLabel(stateLabel) {
			if err := gc.RemoveLabel(org, repo, num, stateLabel); err != nil {
				return fmt.Errorf("error removing label %s from %s/%s #%d: %v", labelName, org, repo, num, err)
			}
		}
	}
	return nil
}

func dayPhrase(days int) string {
	dayString := "days"
	if days == 1 || days == -1 {
		dayString = "day"
	}
	return fmt.Sprintf("%d %s", days, dayString)
}

func durationToMaxDays(duration time.Duration) string {
	days := int(math.Floor(duration.Hours() / 24))
	return dayPhrase(days)
}
