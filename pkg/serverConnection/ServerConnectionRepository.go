package serverConnection

import (
	"github.com/devtron-labs/common-lib/utils/serverConnection/bean"
	"github.com/devtron-labs/kubelink/pkg/sql"
	"github.com/go-pg/pg"
	"go.uber.org/zap"
)

type ServerConnectionRepository interface {
	GetById(id int) (*ServerConnectionConfig, error)
}

type ServerConnectionRepositoryImpl struct {
	logger       *zap.SugaredLogger
	dbConnection *pg.DB
}

func NewServerConnectionRepositoryImpl(dbConnection *pg.DB, logger *zap.SugaredLogger) *ServerConnectionRepositoryImpl {
	return &ServerConnectionRepositoryImpl{
		logger:       logger,
		dbConnection: dbConnection,
	}
}

type ServerConnectionConfig struct {
	tableName        struct{}                    `sql:"server_connection_config" pg:",discard_unknown_columns"`
	Id               int                         `sql:"id,pk"`
	ConnectionMethod bean.ServerConnectionMethod `sql:"connection_method"`
	ProxyUrl         string                      `sql:"proxy_url"`
	SSHServerAddress string                      `sql:"ssh_server_address"`
	SSHUsername      string                      `sql:"ssh_username"`
	SSHPassword      string                      `sql:"ssh_password"`
	SSHAuthKey       string                      `sql:"ssh_auth_key"`
	Deleted          bool                        `sql:"deleted,notnull"`
	sql.AuditLog
}

func (repo *ServerConnectionRepositoryImpl) GetById(id int) (*ServerConnectionConfig, error) {
	model := &ServerConnectionConfig{}
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
