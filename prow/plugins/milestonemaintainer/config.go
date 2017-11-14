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



type milestoneOptName string

const (
	milestoneOptModes                = "milestone-modes"
	milestoneOptWarningInterval      = "milestone-warning-interval"
	milestoneOptLabelGracePeriod     = "milestone-label-grace-period"
	milestoneOptApprovalGracePeriod  = "milestone-approval-grace-period"
	milestoneOptSlushUpdateInterval  = "milestone-slush-update-interval"
	milestoneOptFreezeUpdateInterval = "milestone-freeze-update-interval"
	milestoneOptFreezeDate           = "milestone-freeze-date"
)

	validators map[string]milestoneArgValidator
type milestoneArgValidator func(name string) error


func NewMilestoneMaintainer() *MilestoneMaintainer {
	m := &MilestoneMaintainer{}
	m.validators = map[string]milestoneArgValidator{
		milestoneOptModes: func(name string) error {
			modeMap, err := parseMilestoneModes(m.milestoneModes)
			if err != nil {
				return fmt.Errorf("%s: %s", name, err)
			}
			m.milestoneModeMap = modeMap
			return nil
		},
		milestoneOptWarningInterval: func(name string) error {
			return durationGreaterThanZero(name, m.warningInterval)
		},
		milestoneOptLabelGracePeriod: func(name string) error {
			return durationGreaterThanZero(name, m.labelGracePeriod)
		},
		milestoneOptApprovalGracePeriod: func(name string) error {
			return durationGreaterThanZero(name, m.approvalGracePeriod)
		},
		milestoneOptSlushUpdateInterval: func(name string) error {
			return durationGreaterThanZero(name, m.slushUpdateInterval)
		},
		milestoneOptFreezeUpdateInterval: func(name string) error {
			return durationGreaterThanZero(name, m.freezeUpdateInterval)
		},
		milestoneOptFreezeDate: func(name string) error {
			if len(m.freezeDate) == 0 {
				return fmt.Errorf("%s must be supplied", name)
			}
			return nil
		},
	}
	return m
}
func durationGreaterThanZero(name string, value time.Duration) error {
	if value <= 0 {
		return fmt.Errorf("%s must be greater than zero", name)
	}
	return nil
}


// Initialize will initialize the munger
func (m *MilestoneMaintainer) Initialize(config *github.Config, features *features.Features) error {
	for name, validator := range m.validators {
		if err := validator(name); err != nil {
			return err
		}
	}

	m.botName = config.BotName
	m.features = features
	return nil
}
