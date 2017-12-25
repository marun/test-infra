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

package milestonemaintainer

import (
	"time"

	"k8s.io/test-infra/prow/github"
)

type githubClient interface {
	BotName() (string, error)
	ListIssueComments(org, repo string, number int) ([]github.IssueComment, error)
	ListIssueEvents(org, repo string, num int) ([]github.ListedIssueEvent, error)
	ListPullRequestComments(org, repo string, number int) ([]github.ReviewComment, error)
}

type githubObject struct {
	org       string
	repo      string
	id        int
	createdAt time.Time
	isPR      bool
}

func lastHumanPullRequestUpdate(gh githubClient, botName string, obj githubObject) (*time.Time, error) {
	comments, err := gh.ListPullRequestComments(obj.org, obj.repo, obj.id)
	if err != nil {
		return nil, err
	}

	lastHuman := obj.createdAt
	for i := range comments {
		comment := comments[i]
		if comment.User.Login == botName {
			continue
		}
		if lastHuman.Before(*comment.UpdatedAt) {
			lastHuman = comment.UpdatedAt
		}
	}

	return lastHuman, nil
}

func lastHumanIssueUpdate(gh githubClient, botName string, obj githubObject) (*time.Time, error) {
	comments, err := gh.ListIssueComments(obj.org, obj.repo, obj.id)
	if err != nil {
		return nil, err
	}

	lastHuman := obj.createdAt
	for i := range comments {
		comment := comments[i]
		if comment.User.Login == botName {
			continue
		}
		if lastHuman.Before(*comment.UpdatedAt) {
			lastHuman = comment.UpdatedAt
		}
	}

	return lastHuman, nil
}

func lastInterestingEventUpdate(gh githubClient, botName string, obj githubObject) (*time.Time, error) {
	events, err := gh.ListIssueEvents(obj.org, obj.repo, obj.id)
	if err != nil {
		return nil, err
	}

	lastInteresting := obj.createdAt
	for i := range events {
		event := events[i]
		if event.Event != github.IssueActionReopened {
			continue
		}

		if lastInteresting.Before(event.CreatedAt) {
			lastInteresting = event.CreatedAt
		}
	}

	return lastInteresting, nil
}

func lastModificationTime(gh githubClient, obj githubObject) (*time.Time, error) {
	botName, err := gh.BotName()
	if err != nil {
		return nil, err
	}

	lastHumanIssue, err := lastHumanIssueUpdate(gh, botName, obj)
	if err != nil {
		return nil, err
	}

	lastInterestingEvent, err := lastInterestingEventUpdate(gh, botName, obj)
	if err != nil {
		return nil, err
	}

	var lastModif *time.Time
	lastModif = lastHumanIssue

	if lastInterestingEvent.After(*lastModif) {
		lastModif = lastInterestingEvent
	}

	if obj.isPR {
		lastHumanPR, err := lastHumanPullRequestUpdate(gh, botName, obj)
		if err != nil {
			return nil, err
		}

		if lastHumanPR.After(*lastModif) {
			lastModif = lastHumanPR
		}
	}

	return lastModif, nil
}
