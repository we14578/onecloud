package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"yunion.io/x/jsonutils"
	"yunion.io/x/pkg/util/stringutils"
	"yunion.io/x/sqlchemy"

	"yunion.io/x/onecloud/pkg/cloudcommon/db/lockman"
	"yunion.io/x/onecloud/pkg/httperrors"
	"yunion.io/x/onecloud/pkg/mcclient"
)

const (
	SYSTEM_ADMIN_PREFIX = "_"
	CLOUD_TAG_PREFIX    = "ext:"
)

type SMetadataManager struct {
	SModelBaseManager
}

type SMetadata struct {
	SModelBase

	Id        string    `width:"128" charset:"ascii" primary:"true" list:"user" get:"user"` // = Column(VARCHAR(128, charset='ascii'), primary_key=True)
	Key       string    `width:"64" charset:"utf8" primary:"true" list:"user" get:"user"`   // = Column(VARCHAR(64, charset='ascii'),  primary_key=True)
	Value     string    `charset:"utf8" list:"user" get:"user"`                             // = Column(TEXT(charset='utf8'), nullable=True)
	UpdatedAt time.Time `nullable:"false" updated_at:"true"`                                // = Column(DateTime, default=get_utcnow, nullable=False, onupdate=get_utcnow)
}

var Metadata *SMetadataManager
var ResourceMap map[string]*SVirtualResourceBaseManager

func init() {
	Metadata = &SMetadataManager{SModelBaseManager: NewModelBaseManager(SMetadata{}, "metadata_tbl", "metadata", "metadatas")}
	ResourceMap = map[string]*SVirtualResourceBaseManager{
		"disk":     {SStatusStandaloneResourceBaseManager: NewStatusStandaloneResourceBaseManager(SVirtualResourceBase{}, "disks_tbl", "disk", "disks")},
		"server":   {SStatusStandaloneResourceBaseManager: NewStatusStandaloneResourceBaseManager(SVirtualResourceBase{}, "guests_tbl", "server", "servers")},
		"eip":      {SStatusStandaloneResourceBaseManager: NewStatusStandaloneResourceBaseManager(SVirtualResourceBase{}, "elasticips_tbl", "eip", "eips")},
		"snapshot": {SStatusStandaloneResourceBaseManager: NewStatusStandaloneResourceBaseManager(SVirtualResourceBase{}, "snapshots_tbl", "snpashot", "snpashots")},
	}
}

func (m *SMetadata) GetId() string {
	return fmt.Sprintf("%s-%s", m.Id, m.Key)
}

func (m *SMetadata) GetName() string {
	return fmt.Sprintf("%s-%s", m.Id, m.Key)
}

func (m *SMetadata) GetModelManager() IModelManager {
	return Metadata
}

func GetObjectIdstr(model IModel) string {
	return fmt.Sprintf("%s::%s", model.GetModelManager().Keyword(), model.GetId())
}

func (manager *SMetadataManager) AllowListItems(ctx context.Context, userCred mcclient.TokenCredential, query jsonutils.JSONObject) bool {
	return true
}

func (manager *SMetadataManager) ListItemFilter(ctx context.Context, q *sqlchemy.SQuery, userCred mcclient.TokenCredential, query jsonutils.JSONObject) (*sqlchemy.SQuery, error) {
	resources := jsonutils.GetQueryStringArray(query, "resources")
	if len(resources) == 0 {
		for resource := range ResourceMap {
			resources = append(resources, resource)
		}
	}
	conditions := []sqlchemy.ICondition{}
	admin := jsonutils.QueryBoolean(query, "admin", false)
	for _, resource := range resources {
		if manager, ok := ResourceMap[resource]; ok {
			resourceView := manager.Query().SubQuery()
			prefix := sqlchemy.NewStringField(fmt.Sprintf("%s::", manager.Keyword()))
			field := sqlchemy.CONCAT(manager.Keyword(), prefix, resourceView.Field("id"))
			sq := resourceView.Query(field)
			if !admin || !IsAdminAllowList(userCred, manager) {
				ownerId := manager.GetOwnerId(userCred)
				if len(ownerId) > 0 {
					sq = manager.FilterByOwner(sq, ownerId)
				}
			}
			conditions = append(conditions, sqlchemy.In(q.Field("id"), sq))
		} else {
			return nil, httperrors.NewInputParameterError("Not support resource %s tag filter", resource)
		}
	}
	if len(conditions) > 0 {
		q = q.Filter(sqlchemy.OR(conditions...))
	}
	if !jsonutils.QueryBoolean(query, "with_sys", false) {
		q = q.Filter(sqlchemy.NOT(sqlchemy.Startswith(q.Field("key"), SYSTEM_ADMIN_PREFIX)))
	}
	if !jsonutils.QueryBoolean(query, "with_cloud", false) {
		q = q.Filter(sqlchemy.NOT(sqlchemy.Startswith(q.Field("key"), CLOUD_TAG_PREFIX)))
	}
	return q, nil
}

/* @classmethod
def get_object_idstr(cls, obj, keygen_func):
idstr = None
if keygen_func is not None and callable(keygen_func):
idstr = keygen_func(obj)
elif isinstance(obj, SStandaloneResourceBase):
idstr = '%s::%s' % (obj._resource_name_, obj.id)
if idstr is None:
raise Exception('get_object_idstr: failed to generate obj ID')
return idstr */

func (manager *SMetadataManager) GetStringValue(model IModel, key string, userCred mcclient.TokenCredential) string {
	if strings.HasPrefix(key, SYSTEM_ADMIN_PREFIX) && (userCred == nil || !IsAdminAllowGetSpec(userCred, model, "metadata")) {
		return ""
	}
	idStr := GetObjectIdstr(model)
	m := SMetadata{}
	err := manager.Query().Equals("id", idStr).Equals("key", key).First(&m)
	if err == nil {
		return m.Value
	}
	return ""
}

func (manager *SMetadataManager) GetJsonValue(model IModel, key string, userCred mcclient.TokenCredential) jsonutils.JSONObject {
	if strings.HasPrefix(key, SYSTEM_ADMIN_PREFIX) && (userCred == nil || !IsAdminAllowGetSpec(userCred, model, "metadata")) {
		return nil
	}
	idStr := GetObjectIdstr(model)
	m := SMetadata{}
	err := manager.Query().Equals("id", idStr).Equals("key", key).First(&m)
	if err == nil {
		json, _ := jsonutils.ParseString(m.Value)
		return json
	}
	return nil
}

type sMetadataChange struct {
	Key    string
	OValue string
	NValue string
}

func (manager *SMetadataManager) RemoveAll(ctx context.Context, model IModel, userCred mcclient.TokenCredential) error {
	idStr := GetObjectIdstr(model)
	if len(idStr) == 0 {
		return fmt.Errorf("invalid model")
	}

	lockman.LockObject(ctx, model)
	defer lockman.ReleaseObject(ctx, model)

	records := make([]SMetadata, 0)
	q := manager.Query().Equals("id", idStr)
	err := FetchModelObjects(manager, q, &records)
	if err != nil {
		return fmt.Errorf("find metadata for %s fail: %s", idStr, err)
	}
	changes := make([]sMetadataChange, 0)
	for _, rec := range records {
		if len(rec.Value) > 0 {
			_, err := Update(&rec, func() error {
				rec.Value = ""
				return nil
			})
			if err == nil {
				changes = append(changes, sMetadataChange{Key: rec.Key, OValue: rec.Value})
			}
		}
	}
	if len(changes) > 0 {
		OpsLog.LogEvent(model, ACT_DEL_METADATA, jsonutils.Marshal(changes), userCred)
	}
	return nil
}

func (manager *SMetadataManager) SetValue(ctx context.Context, obj IModel, key string, value interface{}, userCred mcclient.TokenCredential) error {
	return manager.SetAll(ctx, obj, map[string]interface{}{key: value}, userCred)
}

func (manager *SMetadataManager) SetAll(ctx context.Context, obj IModel, store map[string]interface{}, userCred mcclient.TokenCredential) error {
	idStr := GetObjectIdstr(obj)

	lockman.LockObject(ctx, obj)
	defer lockman.ReleaseObject(ctx, obj)

	changes := make([]sMetadataChange, 0)
	for key, value := range store {
		if strings.HasPrefix(key, SYSTEM_ADMIN_PREFIX) && (userCred == nil || !IsAdminAllowGetSpec(userCred, obj, "metadata")) {
			return httperrors.NewForbiddenError("Ordinary users can't set the tags that begin with an underscore")
		}

		valStr := stringutils.Interface2String(value)
		valStrLower := strings.ToLower(valStr)
		if valStrLower == "none" || valStrLower == "null" {
			valStr = ""
		}
		record := SMetadata{}
		err := manager.Query().Equals("id", idStr).Equals("key", key).First(&record)
		if err != nil {
			if err == sql.ErrNoRows {
				changes = append(changes, sMetadataChange{Key: key, NValue: valStr})
				record.Id = idStr
				record.Key = key
				record.Value = valStr
				err = manager.TableSpec().Insert(&record)
				if err != nil {
					return err
				}
			} else {
				return err
			}
		} else {
			_, err := Update(&record, func() error {
				record.Value = valStr
				return nil
			})
			if err != nil {
				return err
			}
			changes = append(changes, sMetadataChange{Key: key, OValue: record.Value, NValue: valStr})
		}
	}
	if len(changes) > 0 {
		OpsLog.LogEvent(obj, ACT_SET_METADATA, jsonutils.Marshal(changes), userCred)
	}
	return nil
}

func (manager *SMetadataManager) GetAll(obj IModel, keys []string, userCred mcclient.TokenCredential) (map[string]string, error) {
	idStr := GetObjectIdstr(obj)
	records := make([]SMetadata, 0)
	q := manager.Query().Equals("id", idStr)
	if keys != nil && len(keys) > 0 {
		q = q.In("key", keys)
	}
	err := FetchModelObjects(manager, q, &records)
	if err != nil {
		return nil, err
	}
	ret := make(map[string]string)
	for _, rec := range records {
		if len(rec.Value) > 0 {
			if strings.HasPrefix(rec.Key, SYSTEM_ADMIN_PREFIX) {
				if userCred != nil && IsAdminAllowGetSpec(userCred, obj, "metadata") {
					key := rec.Key[len(SYSTEM_ADMIN_PREFIX):]
					ret[key] = rec.Value
				}
			} else {
				ret[rec.Key] = rec.Value
			}
		}
	}
	return ret, nil
}

func (manager *SMetadataManager) IsSystemAdminKey(key string) bool {
	return strings.HasPrefix(key, SYSTEM_ADMIN_PREFIX)
}

func (manager *SMetadataManager) GetSysadminKey(key string) string {
	return fmt.Sprintf("%s%s", SYSTEM_ADMIN_PREFIX, key)
}

/*

@classmethod
def get_sysadmin_key_object_ids(cls, obj_cls, key):
sys_key = cls.get_sysadmin_key(key)
ids = Metadata.query(Metadata.id).filter(Metadata.key==sys_key) \
.filter(Metadata.value!=None) \
.filter(Metadata.id.like('%s::%%' % obj_cls._resource_name_)) \
.all()
ret = []
for id, in ids:
ret.append(id[len(obj_cls._resource_name_)+2:])
return ret

*/
