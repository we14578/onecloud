// Copyright 2019 Yunion
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package meter

import (
	"yunion.io/x/onecloud/cmd/climc/shell/events"
	"yunion.io/x/onecloud/pkg/mcclient"
	"yunion.io/x/onecloud/pkg/mcclient/modules"
)

func doMeterEventList(s *mcclient.ClientSession, args *events.EventListOptions) error {
	return events.DoEventList(modules.MeterLogs, s, args)
}

func init() {
	R(&events.EventListOptions{}, "meter-event-show", "Show operation meter event logs", doMeterEventList)
}
