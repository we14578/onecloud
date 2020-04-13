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

package tasks

import (
	"context"

	"yunion.io/x/jsonutils"
	"yunion.io/x/pkg/errors"

	api "yunion.io/x/onecloud/pkg/apis/cloudid"
	"yunion.io/x/onecloud/pkg/cloudcommon/db"
	"yunion.io/x/onecloud/pkg/cloudcommon/db/taskman"
	"yunion.io/x/onecloud/pkg/cloudid/models"
)

type ClouduserSyncstatusTask struct {
	taskman.STask
}

func init() {
	taskman.RegisterTask(ClouduserSyncstatusTask{})
}

func (self *ClouduserSyncstatusTask) taskFailed(ctx context.Context, clouduser *models.SClouduser, err error) {
	clouduser.SetStatus(self.GetUserCred(), api.CLOUD_USER_STATUS_UNKNOWN, err.Error())
	self.SetStageFailed(ctx, err.Error())
}

func (self *ClouduserSyncstatusTask) OnInit(ctx context.Context, obj db.IStandaloneModel, body jsonutils.JSONObject) {
	clouduser := obj.(*models.SClouduser)

	_, err := clouduser.GetIClouduser()
	if err != nil {
		self.taskFailed(ctx, clouduser, errors.Wrap(err, "GetIClouduser"))
		return
	}

	clouduser.SetStatus(self.GetUserCred(), api.CLOUD_USER_STATUS_AVAILABLE, "")
	self.SetStageComplete(ctx, nil)
}
