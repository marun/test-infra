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

package main

import (
	"bytes"
	"flag"
	"fmt"
	"io/ioutil"
	"net/url"
	"os/signal"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/plugins"
	"k8s.io/test-infra/prow/plugins/milestonemaintainer"
)

var (
	dryRun = flag.Bool("dry-run", true, "Dry run for testing. Uses API tokens but does not mutate.")

	githubEndpoint  = flag.String("github-endpoint", "https://api.github.com", "GitHub's API endpoint.")
	githubTokenFile = flag.String("github-token-file", "/etc/github/oauth", "Path to the file containing the GitHub OAuth secret.")
)

func main() {
	flag.Parse()

	// TODO
	// logrus.SetFormatter(&logrus.JSONFormatter{})
	logrus.SetLevel(logrus.DebugLevel)

	log := logrus.WithField("plugin", "milestone-maintainer")

	// Ignore SIGTERM so that we don't drop hooks when the pod is removed.
	// We'll get SIGTERM first and then SIGKILL after our graceful termination
	// deadline.
	signal.Ignore(syscall.SIGTERM)

	oauthSecretRaw, err := ioutil.ReadFile(*githubTokenFile)
	if err != nil {
		log.WithError(err).Fatal("Could not read oauth secret file.")
	}
	oauthSecret := string(bytes.TrimSpace(oauthSecretRaw))

	_, err = url.Parse(*githubEndpoint)
	if err != nil {
		log.WithError(err).Fatal("Must specify a valid --github-endpoint URL.")
	}

	var gc *github.Client
	if *dryRun {
		log.Warning("Using DryRun client - no updates will be made to github")
		gc = github.NewDryRunClient(oauthSecret, *githubEndpoint)
	} else {
		gc = github.NewClient(oauthSecret, *githubEndpoint)
	}

	org := "marun"
	repo := "nkube"

	pluginConfig := plugins.MilestoneMaintainer{
		Modes: map[string]string{
			"v1.0": "dev",
			// "v1.1": "dev",
		},
		WarningInterval:      time.Minute * 1,
		LabelGracePeriod:     time.Minute * 2,
		ApprovalGracePeriod:  time.Minute * 2,
		SlushUpdateInterval:  time.Minute * 2,
		FreezeUpdateInterval: time.Minute * 2,
		FreezeDate:           "TBD",
	}

	log = log.WithFields(logrus.Fields{
		"org":  org,
		"repo": repo,
	})

	for milestone, _ := range pluginConfig.Modes {
		issues, err := gc.FindIssues(fmt.Sprintf("repo:%s/%s state:open milestone:%s", org, repo, milestone), "", false)
		if err != nil {
			log.WithError(err).Fatal("Error getting issues for milestone %s.", milestone)
		}
		//		log.Infof("%d issues for milestone %s : %v\n", len(issues), milestone, issues)
		for _, issue := range issues {
			objType := "issue"
			if issue.IsPullRequest() {
				objType = "pull request"
			}
			l := log.WithField(objType, issue.Number)

			// Create synthetic event
			e := github.IssueEvent{
				Action: github.IssueActionOpened,
				Issue:  issue,
				Repo: github.Repo{
					Owner: github.User{
						Name: org,
					},
					Name: repo,
				},
			}

			if err := milestonemaintainer.HandleIssue(gc, l, pluginConfig, e); err != nil {
				log.WithError(err).Error("Error maintaining issue in milestone")
			}
		}
	}
}
