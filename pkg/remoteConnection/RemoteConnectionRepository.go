package remoteConnection

import (
	remoteConnectionBean "github.com/devtron-labs/common-lib/utils/remoteConnection/bean"
	"github.com/devtron-labs/kubelink/pkg/sql"
	"github.com/go-pg/pg"
	"go.uber.org/zap"
)

type RemoteConnectionRepository interface {
	GetById(id int) (*RemoteConnectionConfig, error)
}

type RemoteConnectionRepositoryImpl struct {
	logger       *zap.SugaredLogger
	dbConnection *pg.DB
}

func NewRemoteConnectionRepositoryImpl(dbConnection *pg.DB, logger *zap.SugaredLogger) *RemoteConnectionRepositoryImpl {
	return &RemoteConnectionRepositoryImpl{
		logger:       logger,
		dbConnection: dbConnection,
	}
}

type RemoteConnectionConfig struct {
	tableName        struct{}                                    `sql:"remote_connection_config" pg:",discard_unknown_columns"`
	Id               int                                         `sql:"id,pk"`
	ConnectionMethod remoteConnectionBean.RemoteConnectionMethod `sql:"connection_method"`
	ProxyUrl         string                                      `sql:"proxy_url"`
	SSHServerAddress string                                      `sql:"ssh_server_address"`
	SSHUsername      string                                      `sql:"ssh_username"`
	SSHPassword      string                                      `sql:"ssh_password"`
	SSHAuthKey       string                                      `sql:"ssh_auth_key"`
	Deleted          bool                                        `sql:"deleted,notnull"`
	sql.AuditLog
}

func (repo *RemoteConnectionRepositoryImpl) GetById(id int) (*RemoteConnectionConfig, error) {
	model := &RemoteConnectionConfig{}
	err := repo.dbConnection.Model(model).
		Where("id = ?", id).
		Where("deleted = ?", false).
		Select()
	if err != nil {
		repo.logger.Errorw("error in getting server connection config", "err", err, "id", id)
		return nil, err
	}
	return model, nil
}
